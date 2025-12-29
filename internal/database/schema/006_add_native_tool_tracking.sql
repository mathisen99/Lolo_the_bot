-- Add tracking columns for native tool costs
-- web_search_calls: Number of web search API calls ($0.01 each)
-- code_interpreter_calls: Number of code interpreter containers ($0.03 each)

ALTER TABLE usage_tracking ADD COLUMN web_search_calls INTEGER DEFAULT 0;
ALTER TABLE usage_tracking ADD COLUMN code_interpreter_calls INTEGER DEFAULT 0;
