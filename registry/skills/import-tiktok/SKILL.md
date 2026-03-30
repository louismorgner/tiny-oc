---
name: import-tiktok
description: Scrape TikTok video captions and metadata using Apify
metadata:
  version: "0.1"
  requires_integration: apify
---

# import-tiktok

Scrape content from TikTok videos using the Apify integration.

## How to use

```bash
toc runtime invoke apify run_actor --actorId "clockworks/tiktok-scraper" --input '{"postURLs": ["<url>"]}'
```

## What to do with the output

Extract the video caption/description and any text overlays. Return as clean text.

If used during bootstrap, treat the extracted caption as a writing sample.
