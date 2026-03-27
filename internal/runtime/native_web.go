package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
)

const (
	webFetchTimeout  = 20 * time.Second
	maxWebFetchBytes = 2 * 1024 * 1024
	webFetchUA       = "tiny-oc-toc-native/1.0 (+https://github.com/tiny-oc/toc)"
)

var nativeWebFetchClient = &http.Client{
	Timeout: webFetchTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	},
}

func nativeWebFetch(ctx nativeToolContext, call ToolCall) toolExecution {
	if err := ValidateNetworkPermission(ctx.Manifest, "web", ctx.Agent); err != nil {
		return toolFailure("WebFetch", "", "", err)
	}

	var args struct {
		URL string `json:"url"`
	}
	if err := decodeToolArgs(call.Function.Arguments, &args); err != nil {
		return toolFailure("WebFetch", "", "", err)
	}
	if strings.TrimSpace(args.URL) == "" {
		return toolFailure("WebFetch", "", "", fmt.Errorf("url is required"))
	}

	output, step, err := fetchWebContent(args.URL)
	if err != nil {
		return toolFailure("WebFetch", args.URL, "", err)
	}
	return toolSuccess("WebFetch", step.Path, output, step)
}

func fetchWebContent(rawURL string) (string, Step, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", Step{}, fmt.Errorf("invalid url %q: %w", rawURL, err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", Step{}, fmt.Errorf("unsupported url scheme %q: only http and https are allowed", parsedURL.Scheme)
	}
	if parsedURL.Host == "" {
		return "", Step{}, fmt.Errorf("url %q is missing a host", rawURL)
	}

	req, err := http.NewRequest(http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return "", Step{}, err
	}
	req.Header.Set("User-Agent", webFetchUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/markdown,text/plain,application/json;q=0.9,*/*;q=0.5")

	start := time.Now()
	resp, err := nativeWebFetchClient.Do(req)
	durationMS := time.Since(start).Milliseconds()
	if err != nil {
		return "", Step{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := readWebFetchBody(resp.Body, maxWebFetchBytes)
	if err != nil {
		return "", Step{}, err
	}

	finalURL := resp.Request.URL
	mediaType := detectWebFetchMediaType(resp.Header.Get("Content-Type"), body)
	content, title, err := renderWebFetchContent(mediaType, body, finalURL)
	if err != nil {
		return "", Step{}, err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		snippet := truncateInlineWeb(strings.TrimSpace(content), 240)
		if snippet != "" {
			return "", Step{}, fmt.Errorf("GET %s returned %s: %s", finalURL.String(), resp.Status, snippet)
		}
		return "", Step{}, fmt.Errorf("GET %s returned %s", finalURL.String(), resp.Status)
	}

	output := formatWebFetchOutput(rawURL, finalURL.String(), resp.Status, mediaType, title, content)
	lines := 0
	if output != "" {
		lines = strings.Count(output, "\n") + 1
	}
	return output, Step{
		Type:       "tool",
		Tool:       "WebFetch",
		Path:       finalURL.String(),
		Lines:      lines,
		DurationMS: durationMS,
		Success:    boolPtr(true),
	}, nil
}

func readWebFetchBody(r io.Reader, limit int64) ([]byte, error) {
	limited := io.LimitReader(r, limit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeded %d bytes", limit)
	}
	return data, nil
}

func detectWebFetchMediaType(contentType string, body []byte) string {
	if contentType != "" {
		if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && mediaType != "" {
			return strings.ToLower(mediaType)
		}
	}
	return strings.ToLower(strings.TrimSpace(http.DetectContentType(body)))
}

func renderWebFetchContent(mediaType string, body []byte, pageURL *url.URL) (content string, title string, err error) {
	switch {
	case mediaType == "text/html", mediaType == "application/xhtml+xml":
		return convertHTMLForWebFetch(body, pageURL)
	case mediaType == "text/markdown", mediaType == "text/x-markdown":
		return normalizeWebFetchText(string(body)), "", nil
	case mediaType == "application/json", strings.HasSuffix(mediaType, "+json"):
		return normalizeWebFetchJSON(body), "", nil
	case strings.HasPrefix(mediaType, "text/"):
		return normalizeWebFetchText(string(body)), "", nil
	default:
		if looksLikeMarkdownURL(pageURL) {
			return normalizeWebFetchText(string(body)), "", nil
		}
		return "", "", fmt.Errorf("unsupported content type %q", mediaType)
	}
}

func convertHTMLForWebFetch(body []byte, pageURL *url.URL) (string, string, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("failed to parse html: %w", err)
	}

	title := extractHTMLTitle(doc)
	root, bodyFallback := selectWebFetchRoot(doc)
	if root == nil {
		root = doc
	}

	sanitizeWebFetchTree(root, pageURL, bodyFallback)

	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(
				table.WithCellPaddingBehavior(table.CellPaddingBehaviorMinimal),
				table.WithHeaderPromotion(true),
				table.WithSkipEmptyRows(true),
			),
		),
	)
	conv.Register.TagType("nav", converter.TagTypeRemove, converter.PriorityStandard)
	conv.Register.TagType("footer", converter.TagTypeRemove, converter.PriorityStandard)
	conv.Register.TagType("aside", converter.TagTypeRemove, converter.PriorityStandard)
	conv.Register.TagType("form", converter.TagTypeRemove, converter.PriorityStandard)
	conv.Register.TagType("dialog", converter.TagTypeRemove, converter.PriorityStandard)
	conv.Register.TagType("button", converter.TagTypeRemove, converter.PriorityStandard)

	markdown, err := conv.ConvertNode(root)
	if err != nil {
		return "", title, fmt.Errorf("failed to convert html to markdown: %w", err)
	}

	content := normalizeWebFetchText(string(markdown))
	if content == "" {
		return "", title, fmt.Errorf("page did not contain readable content")
	}
	return content, title, nil
}

func sanitizeWebFetchTree(node *html.Node, pageURL *url.URL, removePageChrome bool) {
	if node == nil {
		return
	}
	absolutizeNodeAttrs(node, pageURL)

	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		if shouldRemoveWebFetchNode(child, removePageChrome) {
			node.RemoveChild(child)
			child = next
			continue
		}
		sanitizeWebFetchTree(child, pageURL, removePageChrome)
		child = next
	}
}

func shouldRemoveWebFetchNode(node *html.Node, removePageChrome bool) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}

	name := strings.ToLower(node.Data)
	if isAlwaysRemovedWebNode(name) {
		return true
	}
	if removePageChrome && isPageChromeWebNode(name) {
		return true
	}

	for _, attr := range node.Attr {
		key := strings.ToLower(attr.Key)
		value := strings.ToLower(strings.TrimSpace(attr.Val))
		if key == "hidden" {
			return true
		}
		if key == "aria-hidden" && value == "true" {
			return true
		}
	}

	return false
}

func isAlwaysRemovedWebNode(name string) bool {
	switch name {
	case "script", "style", "noscript", "svg", "canvas", "template", "iframe":
		return true
	default:
		return false
	}
}

func isPageChromeWebNode(name string) bool {
	switch name {
	case "header", "nav", "footer", "aside":
		return true
	default:
		return false
	}
}

func absolutizeNodeAttrs(node *html.Node, pageURL *url.URL) {
	if node == nil || node.Type != html.ElementNode || pageURL == nil {
		return
	}
	for i := range node.Attr {
		key := strings.ToLower(node.Attr[i].Key)
		switch key {
		case "href", "src", "poster", "cite", "action":
			node.Attr[i].Val = absolutizeURLAttr(node.Attr[i].Val, pageURL)
		}
	}
}

func absolutizeURLAttr(raw string, pageURL *url.URL) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || pageURL == nil {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return pageURL.ResolveReference(ref).String()
}

func selectWebFetchRoot(doc *html.Node) (*html.Node, bool) {
	if node := findFirstHTMLElement(doc, func(n *html.Node) bool {
		return strings.EqualFold(n.Data, "main")
	}); node != nil {
		return node, false
	}
	if node := findFirstHTMLElement(doc, func(n *html.Node) bool {
		return hasHTMLAttr(n, "role", "main")
	}); node != nil {
		return node, false
	}
	if node := findFirstHTMLElement(doc, func(n *html.Node) bool {
		return strings.EqualFold(n.Data, "article")
	}); node != nil {
		return node, false
	}
	if node := findFirstHTMLElement(doc, func(n *html.Node) bool {
		return strings.EqualFold(n.Data, "body")
	}); node != nil {
		return node, true
	}
	return doc, true
}

func findFirstHTMLElement(node *html.Node, match func(*html.Node) bool) *html.Node {
	if node == nil {
		return nil
	}
	if node.Type == html.ElementNode && match(node) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirstHTMLElement(child, match); found != nil {
			return found
		}
	}
	return nil
}

func hasHTMLAttr(node *html.Node, key, want string) bool {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) && strings.EqualFold(strings.TrimSpace(attr.Val), want) {
			return true
		}
	}
	return false
}

func extractHTMLTitle(doc *html.Node) string {
	titleNode := findFirstHTMLElement(doc, func(n *html.Node) bool {
		return strings.EqualFold(n.Data, "title")
	})
	if titleNode == nil {
		return ""
	}
	return normalizeWebFetchText(htmlNodeText(titleNode))
}

func htmlNodeText(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var b strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		b.WriteString(htmlNodeText(child))
	}
	return b.String()
}

func formatWebFetchOutput(requestURL, finalURL, status, mediaType, title, content string) string {
	var b strings.Builder
	b.WriteString("URL: ")
	b.WriteString(requestURL)
	b.WriteString("\n")
	if finalURL != "" && finalURL != requestURL {
		b.WriteString("Final URL: ")
		b.WriteString(finalURL)
		b.WriteString("\n")
	}
	if status != "" {
		b.WriteString("Status: ")
		b.WriteString(status)
		b.WriteString("\n")
	}
	if mediaType != "" {
		b.WriteString("Content-Type: ")
		b.WriteString(mediaType)
		b.WriteString("\n")
	}
	if title != "" {
		b.WriteString("Title: ")
		b.WriteString(title)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(strings.TrimSpace(content))
	return truncateToolOutput("WebFetch", b.String())
}

func normalizeWebFetchText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

func normalizeWebFetchJSON(body []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err == nil {
		return normalizeWebFetchText(buf.String())
	}
	return normalizeWebFetchText(string(body))
}

func looksLikeMarkdownURL(pageURL *url.URL) bool {
	if pageURL == nil {
		return false
	}
	path := strings.ToLower(pageURL.Path)
	return strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".markdown") || strings.HasSuffix(path, ".mdx")
}

func truncateInlineWeb(s string, maxLen int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
