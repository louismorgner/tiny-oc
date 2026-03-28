package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tiny-oc/toc/internal/inspect"
	"github.com/tiny-oc/toc/internal/session"
)

func InspectDir(workspace, sessionID string) string {
	return filepath.Join(MetadataDir(workspace, sessionID), "inspect")
}

func InspectCapturePathInWorkspace(workspace, sessionID string) string {
	return filepath.Join(InspectDir(workspace, sessionID), "http.jsonl")
}

func InspectCapturePath(sess *session.Session) string {
	if dir := sess.MetadataDirPath(); dir != "" {
		return filepath.Join(dir, "inspect", "http.jsonl")
	}
	return ""
}

func InspectProxyReadyPathInWorkspace(workspace, sessionID string) string {
	return filepath.Join(InspectDir(workspace, sessionID), "proxy-ready.txt")
}

func InspectProxyStderrPathInWorkspace(workspace, sessionID string) string {
	return filepath.Join(InspectDir(workspace, sessionID), "proxy.stderr.log")
}

func InspectProxyStderrPath(sess *session.Session) string {
	if dir := sess.MetadataDirPath(); dir != "" {
		return filepath.Join(dir, "inspect", "proxy.stderr.log")
	}
	return ""
}

func helperExecutable() (string, error) {
	if path := os.Getenv("TOC_HELPER_EXECUTABLE"); path != "" {
		return path, nil
	}
	if path := os.Getenv("TOC_NATIVE_EXECUTABLE"); path != "" {
		return path, nil
	}
	return os.Executable()
}

func startInspectorProcess(upstreamBaseURL, workspace, sessionID string) (*inspect.HelperProcess, error) {
	helperExe, err := helperExecutable()
	if err != nil {
		return nil, err
	}
	return inspect.StartHelper(inspect.HelperOptions{
		Executable:      helperExe,
		UpstreamBaseURL: upstreamBaseURL,
		CapturePath:     InspectCapturePathInWorkspace(workspace, sessionID),
		ReadyPath:       InspectProxyReadyPathInWorkspace(workspace, sessionID),
		StderrPath:      InspectProxyStderrPathInWorkspace(workspace, sessionID),
	})
}

func inspectShellSetup(helperExecutable, upstreamBaseURL, workspace, sessionID, envName string) string {
	inspectDir := InspectDir(workspace, sessionID)
	readyPath := InspectProxyReadyPathInWorkspace(workspace, sessionID)
	capturePath := InspectCapturePathInWorkspace(workspace, sessionID)
	stderrPath := InspectProxyStderrPathInWorkspace(workspace, sessionID)
	return fmt.Sprintf(`cleanup_proxy() {
if [ -n "${TOC_INSPECT_PROXY_PID:-}" ]; then
kill "$TOC_INSPECT_PROXY_PID" >/dev/null 2>&1 || true
wait "$TOC_INSPECT_PROXY_PID" >/dev/null 2>&1 || true
fi
}
mkdir -p %q
rm -f %q
%q %s --listen-addr 127.0.0.1:0 --upstream %q --capture-file %q --ready-file %q >/dev/null 2>>%q &
TOC_INSPECT_PROXY_PID=$!
trap cleanup_proxy EXIT
while [ ! -s %q ]; do
if ! kill -0 "$TOC_INSPECT_PROXY_PID" >/dev/null 2>&1; then
echo "inspect proxy failed to start" >&2
exit 1
fi
sleep 0.05
done
export %s="$(cat %q)"
`, inspectDir, readyPath, helperExecutable, inspect.HelperCommandName, upstreamBaseURL, capturePath, readyPath, stderrPath, readyPath, envName, readyPath)
}

func resolveClaudeBaseURLFromEnv() string {
	v := ""
	if env := os.Getenv("ANTHROPIC_BASE_URL"); env != "" {
		v = env
	}
	if v == "" {
		v = inspect.DefaultClaudeBaseURL
	}
	return trimBaseURL(v)
}

func trimBaseURL(v string) string {
	return strings.TrimRight(v, "/")
}
