# qself [![Build Status](https://github.com/brandur/qself/workflows/qself%20CI/badge.svg)](https://github.com/brandur/qself/actions)

Qself is a small tool to sync personal data from APIs down to local TOML files for easier portability and storage.

## Services

### All

    qself sync-all \
        --goodreads-path data/goodreads.toml \
        --twitter-path data/twitter.toml

Requires **all** the env specified in each service below.

### Goodreads

    qself sync-goodreads data/goodreads.toml

Required env:

* `GOODREADS_ID`: ID of the user whose reviews to sync.
* `GOODREADS_KEY`: Goodreads API key.

### Twitter

    qself sync-twitter data/twitter.toml

Required env:

* `TWITTER_CONSUMER_KEY`: OAuth application consumer key.
* `TWITTER_CONSUMER_SECRET`: OAuth application consumer secret.
* `TWITTER_ACCESS_TOKEN`: Access token.
* `TWITTER_ACCESS_SECRET`: Access token secret.
* `TWITTER_USER`: Nickname of user whose data to sync.
