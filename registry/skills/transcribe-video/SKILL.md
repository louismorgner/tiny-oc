---
name: transcribe-video
description: Transcribe video content from URLs (YouTube, Instagram, TikTok, etc.) using Apify actors
metadata:
  version: "0.1"
  requires_integration: apify
---

# transcribe-video

You can transcribe video content from URLs. When a user pastes a video link, extract the transcript using the Apify integration.

## How to use

```bash
toc runtime invoke apify run_actor --actorId "<actor>" --input '{"url": "<video_url>"}'
```

Pick the right actor for the platform. Common actors:

- **YouTube**: `bernardo/youtube-transcript-scraper` — input: `{"urls": ["<url>"]}`
- **Instagram**: `apify/instagram-scraper` — input: `{"directUrls": ["<url>"], "resultsType": "posts"}`
- **TikTok**: `clockworks/tiktok-scraper` — input: `{"postURLs": ["<url>"]}`

If you encounter a platform you don't have an actor for, search Apify's store mentally for the right one. The actor ID format is `username/actor-name`.

## What to do with the output

The result is raw data from the actor. Extract the text content — transcript, caption, description — and return it as clean text. Strip metadata the user doesn't need.

If this is being used during bootstrap to learn voice, treat the extracted text as a writing sample.
