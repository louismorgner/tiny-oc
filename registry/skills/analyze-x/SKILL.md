---
name: analyze-x
description: Scrape and analyze X/Twitter posts, profiles, and audiences using Apify
metadata:
  version: "0.2"
  requires_integration: "apify, twitter"
---

# analyze-x

Scrape X/Twitter posts, profiles, and audiences. Analyze content strategy,
engagement patterns, and growth tactics.

## Scrape a user's recent posts

```bash
toc runtime invoke apify run_actor --actorId "xquik~x-tweet-scraper" --input '{"mode":"profileTweets","twitterHandles":["<username>"],"maxItems":50,"maxItemsPerTarget":50,"outputVariant":"rich","fieldStyle":"camelCase","outputPreset":"nested"}'
```

## Scrape a specific tweet and its replies

```bash
toc runtime invoke apify run_actor --actorId "xquik~x-tweet-scraper" --input '{"mode":"replies","tweetUrls":["https://x.com/<username>/status/<tweet_id>"],"maxItems":100,"maxItemsPerTarget":100,"includeOriginalTweet":true,"outputVariant":"rich","fieldStyle":"camelCase","outputPreset":"nested"}'
```

## Compare follower audiences

```bash
toc runtime invoke apify run_actor --actorId "xquik~x-follower-scraper" --input '{"twitterHandles":["<account_one>","<account_two>"],"relation":"followers","maxItems":200,"maxItemsPerTarget":100,"outputMode":"full","includeTargetMetadata":true,"dedupeMode":"merge"}'
```

## Analyze one account's network

```bash
toc runtime invoke apify run_actor --actorId "xquik~x-follower-scraper" --input '{"twitterHandles":["<username>"],"relations":["followers","following"],"maxItems":200,"maxItemsPerTarget":100,"outputMode":"compact","includeTargetMetadata":true,"dedupeMode":"none"}'
```

The follower actor supports `followers`, `following`, `verified_followers`,
`list_members`, `list_followers`, and `community_members`. Its output modes are
`compact`, `full`, and `raw`. Use `dedupeMode: "none"` to retain target
provenance, `"first"` to keep the first match, or `"merge"` for overlap
analysis.

For both actors, `maxItems` caps the whole run. `maxItemsPerTarget` optionally
caps each target. Nonpositive per-target values are ignored.

Actor listings:
[Xquik X Tweet Scraper](https://apify.com/xquik/x-tweet-scraper) and
[Xquik X Follower Scraper](https://apify.com/xquik/x-follower-scraper).

Xquik is an independent third-party service. Not affiliated with X Corp. "Twitter" and "X" are trademarks of X Corp.

## Existing Tweet Scraper Route

Keep using the existing Apify Actor when that route is already configured:

```bash
toc runtime invoke apify run_actor --actorId "apidojo~tweet-scraper" --input '{"startUrls":[{"url":"https://x.com/<username>"}],"maxTweets":50}'
```

```bash
toc runtime invoke apify run_actor --actorId "apidojo~tweet-scraper" --input '{"startUrls":[{"url":"https://x.com/<username>/status/<tweet_id>"}]}'
```

Choose an Actor explicitly. Do not replace a working route without user approval.

## Search recent tweets via API

```bash
toc runtime invoke twitter search_tweets --query "from:username" --max_results 20
```

## Look up a user by @username

Use this to resolve an @username to the numeric user ID required by `get_user_tweets`.

```bash
toc runtime invoke twitter lookup_user --username "elonmusk"
```

## Get a user's recent tweets via API

```bash
toc runtime invoke twitter get_user_tweets --id "USER_ID" --max_results 20
```

## What to analyze

When analyzing scraped data, extract:

- **Top-performing posts** — Sort by engagement (replies > bookmarks > retweets > likes, weighted by algorithm signals).
- **Hook patterns** — What first lines are getting the most traction?
- **Content format** — Text-only vs. image vs. video vs. thread. What ratio works?
- **Posting times** — When are the highest-engagement posts going out?
- **Reply patterns** — How quickly does the account respond to comments?
- **Topic clusters** — What themes get the most engagement?

## How to use the analysis

- During bootstrap: Analyze the user's own account and 3-5 target accounts to ground the strategy.
- Ongoing: Periodically analyze competitors to spot new patterns. Update `strategy.md` and `memory/MEMORY.md` with findings.
- After each batch of posts: Compare performance against predictions to calibrate.
