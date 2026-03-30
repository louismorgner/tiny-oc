---
name: scrape-text
description: Extract text content from web URLs (articles, tweets, blog posts, bios) using Apify actors
metadata:
  version: "0.1"
  requires_integration: apify
---

# scrape-text

You can extract text from web pages. When a user pastes a link to an article, tweet, blog post, or any text-based page, scrape the content using the Apify integration.

## How to use

```bash
toc runtime invoke apify run_actor --actorId "<actor>" --input '{"url": "<url>"}'
```

Pick the right actor for the content type:

- **Twitter/X**: `apidojo/tweet-scraper` — input: `{"startUrls": [{"url": "<url>"}]}`
- **LinkedIn**: `anchor/linkedin-scraper` — input: `{"urls": ["<url>"]}`
- **Generic article/blog**: `apify/website-content-crawler` — input: `{"startUrls": [{"url": "<url>"}], "maxPagesPerCrawl": 1}`

For anything else, `apify/website-content-crawler` is a good default — it extracts readable text from any page.

## What to do with the output

Extract the text body. Strip navigation, ads, boilerplate. Return clean, readable text.

If this is being used during bootstrap to learn voice, treat the extracted text as a writing sample.
