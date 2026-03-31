---
name: post-to-x
description: Post tweets and replies to X/Twitter using the Twitter API v2
metadata:
  version: "0.1"
  requires_integration: twitter
---

# post-to-x

Post tweets and replies directly to X/Twitter.

## Post a tweet

```bash
toc runtime invoke twitter post_tweet --text "Your tweet text here"
```

## Reply to a tweet

```bash
toc runtime invoke twitter post_tweet --text "Your reply text" --reply_to "1234567890"
```

## Quote tweet

```bash
toc runtime invoke twitter post_tweet --text "Your commentary" --quote_tweet_id "1234567890"
```

## Guidelines

- Always get user approval before posting. Never post without explicit confirmation.
- Check character count: 280 for free accounts, 10,000 for Premium, 25,000 for Premium+.
- If the post includes a link, put it in a reply, not the main tweet.
- After posting, save the draft to `drafts/` with the tweet ID in frontmatter for tracking.
