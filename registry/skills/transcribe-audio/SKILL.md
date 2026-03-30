---
name: transcribe-audio
description: Transcribe local audio/video files or direct media URLs using the Groq speech-to-text API
metadata:
  version: "0.1"
  requires_integration: groq
---

# transcribe-audio

Use this when the user gives you a local media file or a direct audio/video file URL and wants a transcript.

This does not replace `transcribe-video`. That skill is for platform pages like YouTube, TikTok, or Instagram where you need a scraper or extractor first. Use `transcribe-audio` when you already have the media file itself.

## How to use

For a local file:

```bash
toc runtime invoke groq audio.transcribe --file "<path>" --model whisper-large-v3-turbo
```

For a direct media URL:

```bash
toc runtime invoke groq audio.transcribe --url "<direct_media_url>" --model whisper-large-v3-turbo
```

Use `--response_format verbose_json` when you need timestamps. If you want word timestamps too, add:

```bash
--timestamp_granularities word,segment
```

If the audio is not English and you know the language, pass `--language <iso-639-1>` to improve accuracy and latency.

For spoken audio that should be translated into English instead of transcribed verbatim:

```bash
toc runtime invoke groq audio.translate --file "<path>" --model whisper-large-v3
```

## What to return

Return the transcript as clean text by default. If the user asks for timestamps, speaker notes, or a structured summary, use `verbose_json` and format the output accordingly.
