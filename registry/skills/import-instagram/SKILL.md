---
name: import-instagram
description: Scrape Instagram profiles, posts, and reels using Apify
metadata:
  version: "0.1"
  requires_integration: apify
---

# import-instagram

Scrape content from Instagram — profiles, posts, reels, hashtags — using the Apify integration.

## How to use

```bash
toc runtime invoke apify run_actor --actorId "apidojo~instagram-scraper-api" --input '<input_json>'
```

The actor auto-detects what to scrape based on the URL. Pass URLs in `startUrls`:

**Profile** — gets bio, stats, and recent posts:
```json
{"startUrls": ["https://www.instagram.com/username/"], "maxItems": 20}
```

**Single post or reel:**
```json
{"startUrls": ["https://www.instagram.com/p/ABC123/"], "maxItems": 1}
```

**Reel:**
```json
{"startUrls": ["https://www.instagram.com/reel/ABC123/"], "maxItems": 1}
```

**Hashtag:**
```json
{"startUrls": ["https://www.instagram.com/explore/tags/topic/"], "maxItems": 50}
```

Use `maxItems` to cap results and control cost.

## What to do with the output

Extract captions, bios, and any text content. For posts/reels, pull the caption. For profiles, pull the bio and recent post captions.

If used during bootstrap, treat extracted captions and bios as writing samples.
