-- v12 (compatible with v8+): Add sender timestamp and prune index for privacy tokens
ALTER TABLE wanoise_privacy_tokens ADD COLUMN sender_timestamp BIGINT;

CREATE INDEX idx_wanoise_privacy_tokens_our_jid_timestamp
ON wanoise_privacy_tokens (our_jid, timestamp);
