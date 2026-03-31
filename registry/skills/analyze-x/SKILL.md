---
name: analyze-x
description: Scrape and analyze X/Twitter profiles and tweets using Apify
metadata:
  version: "0.1"
  requires_integration: [apify, twitter]
---

# analyze-x

Scrape X/Twitter accounts and tweets to analyze content strategy, engagement patterns, and growth tactics.

## Scrape a user's recent tweets

```bash
toc runtime invoke apify run_actor --actorId "apidojo/tweet-scraper" --input '{"startUrls": [{"url": "https://x.com/<username>"}], "maxTweets": 50}'
```

## Scrape a specific tweet and its replies

```bash
toc runtime invoke apify run_actor --actorId "apidojo/tweet-scraper" --input '{"startUrls": [{"url": "https://x.com/<username>/status/<tweet_id>"}]}'
```

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
