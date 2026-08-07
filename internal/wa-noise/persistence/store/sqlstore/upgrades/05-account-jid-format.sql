-- v5: Update account JID format
UPDATE wanoise_device SET jid=REPLACE(jid, '.0', '');
