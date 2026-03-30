---
name: import-linkedin
description: Scrape LinkedIn posts and profiles using Apify
metadata:
  version: "0.1"
  requires_integration: apify
---

# import-linkedin

Scrape content from LinkedIn — posts and profiles — using the Apify integration.

## How to use

```bash
toc runtime invoke apify run_actor --actorId "anchor/linkedin-scraper" --input '{"urls": ["<url>"]}'
```

## What to do with the output

Extract the post text or profile summary/bio. Strip boilerplate and metadata.

If used during bootstrap, treat extracted content as writing samples.
