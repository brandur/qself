# qself [![Build Status](https://github.com/brandur/qself/workflows/qself%20CI/badge.svg)](https://github.com/brandur/qself/actions)

Qself is a small tool to sync personal data from APIs down to local TOML files for easier portability and storage.

## Services

### Twitter

    qself sync-twitter data/twitter.toml

Required env:

* `TWITTER_CONSUMER_KEY`: OAuth application consumer key.
* `TWITTER_CONSUMER_SECRET`: OAuth application consumer secret.
* `TWITTER_ACCESS_TOKEN`: Access token.
* `TWITTER_ACCESS_SECRET`: Access token secret.
* `TWITTER_USER`: Nickname of user whose data to sync.
