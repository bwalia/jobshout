-- Allow tool-role rows so the chat transcript can store tool calls and
-- results in order (eval M13). Existing user/agent/system rows are unchanged.

ALTER TABLE chat_messages DROP CONSTRAINT IF EXISTS chat_messages_role_check;
ALTER TABLE chat_messages ADD CONSTRAINT chat_messages_role_check
    CHECK (role IN ('user','agent','system','tool'));
