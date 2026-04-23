-- Migration 009: Add per-channel command prefix overrides

ALTER TABLE channel_states ADD COLUMN command_prefix TEXT DEFAULT NULL;
