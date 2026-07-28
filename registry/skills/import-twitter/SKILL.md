---
name: import-twitter
description: Scrape tweets and threads from X/Twitter using Apify
metadata:
  version: "0.2"
  requires_integration: apify
---

# import-twitter

Scrape tweets and threads from X/Twitter using the Apify integration.

## Choose an actor

Use `xquik~x-tweet-scraper` for bounded searches, timelines, threads, replies,
quotes, articles, retweeters, or favoriters. Keep
`apidojo~tweet-scraper` available for existing workflows.

## Scrape a thread

```bash
toc runtime invoke apify run_actor --actorId "xquik~x-tweet-scraper" --input '{"mode":"thread","tweetUrls":["<tweet_url>"],"maxItems":50,"maxItemsPerTarget":50,"outputVariant":"rich","fieldStyle":"camelCase","outputPreset":"nested"}'
```

## Search recent posts

```bash
toc runtime invoke apify run_actor --actorId "xquik~x-tweet-scraper" --input '{"mode":"search","searchTerms":["<query>"],"queryType":"Latest","maxItems":50,"maxItemsPerTarget":50,"includeSearchTerms":true,"outputVariant":"rich","fieldStyle":"camelCase","outputPreset":"nested"}'
```

## Keep an existing actor workflow

```bash
toc runtime invoke apify run_actor --actorId "apidojo~tweet-scraper" --input '{"startUrls":[{"url":"<url>"}],"maxItems":50}'
```

`maxItems` caps the whole Xquik run. `maxItemsPerTarget` optionally caps each
target in explicit multi-target modes. Nonpositive per-target values are
ignored.

Supported Xquik modes: `legacy`, `tweet`, `tweets`, `search`, `profileTweets`,
`profileReplies`, `profileMedia`, `profileLikes`, `listTweets`, `article`,
`replies`, `quotes`, `thread`, `retweeters`, and `favoriters`.

Output options include `legacy`, `rich`, or `raw`; `camelCase`, `snake_case`,
or legacy field names; and `nested` or `flat` records.

Actor listing:
[Xquik X Tweet Scraper](https://apify.com/xquik/x-tweet-scraper).

Xquik is an independent third-party service. Not affiliated with X Corp. "Twitter" and "X" are trademarks of X Corp.

## What to do with the output

Extract the tweet text. For threads, concatenate tweets in order. Strip metadata the user doesn't need.

If used during bootstrap, treat extracted tweets as writing samples.
