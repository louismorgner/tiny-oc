---
name: transcribe-video
description: Transcribe YouTube video content using Apify
metadata:
  version: "0.2"
  requires_integration: apify
---

# transcribe-video

Extract transcripts from YouTube videos using the Apify integration.

## How to use

```bash
toc runtime invoke apify run_actor --actorId "pintostudio~youtube-transcript-scraper" --input '{"videoUrls": ["<url>"], "language": "en"}'
```

## What to do with the output

Extract the transcript text. Return it as clean, readable text — strip timestamps and metadata unless the user asks for them.

If used during bootstrap, treat the transcript as a writing sample.
