---
name: import-twitter
description: Scrape tweets and threads from X/Twitter using Apify
metadata:
  version: "0.1"
  requires_integration: apify
---

# import-twitter

Scrape tweets and threads from X/Twitter using the Apify integration.

## How to use

```bash
toc runtime invoke apify run_actor --actorId "apidojo~tweet-scraper" --input '{"startUrls": [{"url": "<url>"}]}'
```

## What to do with the output

Extract the tweet text. For threads, concatenate tweets in order. Strip metadata the user doesn't need.

If used during bootstrap, treat extracted tweets as writing samples.
