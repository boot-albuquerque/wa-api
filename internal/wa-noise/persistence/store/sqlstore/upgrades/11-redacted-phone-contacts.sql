-- v11 (compatible with v8+): Store redacted phone number for LID members in groups
ALTER TABLE wanoise_contacts ADD COLUMN redacted_phone TEXT;
