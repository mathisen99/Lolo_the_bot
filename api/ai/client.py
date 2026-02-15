"""
AI client for GPT-5.1 integration.

Handles communication with OpenAI API and tool execution.
"""

from typing import List, Dict, Any, Optional
import json
from openai import OpenAI
from .config import AIConfig
from .usage_tracker import log_usage, extract_usage_from_response
from api.tools import WebSearchTool, PythonExecTool, FluxCreateTool, FluxEditTool, ImageAnalysisTool, FetchUrlTool, UserRulesTool, ChatHistoryTool, PasteTool, ShellExecTool, VoiceSpeakTool, NullResponseTool, NULL_RESPONSE_MARKER, BugReportTool, GPTImageTool, GeminiImageTool, UsageStatsTool, ReportStatusTool, YouTubeSearchTool, SourceCodeTool, IRCCommandTool, ClaudeTechTool, STATUS_UPDATE_MARKER, is_image_tool, check_image_rate_limit, record_image_generation, KnowledgeBaseLearnTool, KnowledgeBaseSearchTool, KnowledgeBaseListTool, KnowledgeBaseForgetTool, MoltbookPostTool, ReminderTool
from api.utils.output import log_info, log_error, log_debug, log_success, log_warning


class AIClient:
    """Client for interacting with GPT-5.1 API."""
    
    def __init__(self, config: Optional[AIConfig] = None):
        """
        Initialize AI client.
        
        Args:
            config: AI configuration. If None, loads default config.
        """
        self.config = config or AIConfig()
        self.client = OpenAI(api_key=self.config.openai_api_key)
        self.tools: Dict[str, Any] = {}
        
        # Initialize tools
        self._setup_tools()
    
    def _check_tools_used(self, response: Any, request_id: str) -> None:
        """Check which tools were used in the response and log them."""
        tools_used = []
        output_items = getattr(response, 'output', None)
        
        if output_items:
            for item in output_items:
                item_type = getattr(item, 'type', None)
                if item_type == 'web_search_call':
                    tools_used.append('WEB_SEARCH')
                elif item_type == 'code_interpreter_call':
                    tools_used.append('CODE_INTERPRETER')
                elif item_type == 'function_call':
                    func_name = getattr(item, 'name', None)
                    if func_name == 'flux_create_image':
                        tools_used.append('FLUX_CREATE')
                    elif func_name == 'flux_edit_image':
                        tools_used.append('FLUX_EDIT')
                    elif func_name == 'analyze_image':
                        tools_used.append('IMAGE_ANALYSIS')
                    elif func_name == 'fetch_url':
                        tools_used.append('FETCH_URL')
                    elif func_name == 'manage_user_rules':
                        tools_used.append('USER_RULES')
                    elif func_name == 'query_chat_history':
                        tools_used.append('CHAT_HISTORY')
                    elif func_name == 'create_paste':
                        tools_used.append('PASTE')
                    elif func_name == 'execute_shell':
                        tools_used.append('SHELL_EXEC')
                    elif func_name == 'voice_speak':
                        tools_used.append('VOICE_SPEAK')
                    elif func_name == 'null_response':
                        tools_used.append('NULL_RESPONSE')
                    elif func_name == 'bug_report':
                        tools_used.append('BUG_REPORT')
                    elif func_name == 'gpt_image':
                        tools_used.append('GPT_IMAGE')
                    elif func_name == 'gemini_image':
                        tools_used.append('GEMINI_IMAGE')
                    elif func_name == 'usage_stats':
                        tools_used.append('USAGE_STATS')
                    elif func_name == 'source_code':
                        tools_used.append('SOURCE_CODE')
                    elif func_name == 'irc_command':
                        tools_used.append('IRC_COMMAND')
                    elif func_name == 'claude_tech':
                        tools_used.append('CLAUDE_TECH')
                    elif func_name == 'moltbook_post':
                        tools_used.append('MOLTBOOK_POST')
                    elif func_name == 'reminder':
                        tools_used.append('REMINDER')
        
        if tools_used:
            tools_str = ', '.join(tools_used)
            log_success(f"[{request_id}] 🔧 Tools: {tools_str}")
    
    def _setup_tools(self) -> None:
        """Set up available tools based on configuration."""
        if self.config.web_search_enabled:
            web_search = WebSearchTool(
                external_web_access=self.config.web_search_external_access,
                allowed_domains=self.config.web_search_allowed_domains
            )
            self.tools[web_search.name] = web_search
            log_info("Web search tool enabled")
        
        if self.config.python_exec_enabled:
            python_exec = PythonExecTool()
            self.tools[python_exec.name] = python_exec
            log_info("Python execution tool enabled (Firecracker sandbox)")
        
        if self.config.flux_create_enabled:
            flux_create = FluxCreateTool()
            self.tools[flux_create.name] = flux_create
            log_info("Flux image creation tool enabled")
        
        if self.config.flux_edit_enabled:
            flux_edit = FluxEditTool()
            self.tools[flux_edit.name] = flux_edit
            log_info("Flux image editing tool enabled")
        
        if self.config.image_analysis_enabled:
            image_analysis = ImageAnalysisTool()
            self.tools[image_analysis.name] = image_analysis
            log_info("Image analysis tool enabled")
        
        if self.config.fetch_url_enabled:
            fetch_url = FetchUrlTool()
            self.tools[fetch_url.name] = fetch_url
            log_info("URL fetching tool enabled")
        
        if self.config.user_rules_enabled:
            user_rules = UserRulesTool()
            self.tools[user_rules.name] = user_rules
            log_info("User rules tool enabled")
        
        if self.config.chat_history_enabled:
            chat_history = ChatHistoryTool()
            self.tools[chat_history.name] = chat_history
            log_info("Chat history tool enabled")
        
        if self.config.paste_enabled:
            paste = PasteTool()
            self.tools[paste.name] = paste
            log_info("Paste tool enabled")
        
        if self.config.shell_exec_enabled:
            shell_exec = ShellExecTool(timeout=self.config.shell_exec_timeout)
            self.tools[shell_exec.name] = shell_exec
            log_info("Shell execution tool enabled (OWNER ONLY)")
        
        if self.config.voice_speak_enabled:
            voice_speak = VoiceSpeakTool()
            self.tools[voice_speak.name] = voice_speak
            log_info("Voice speak tool enabled")
        
        if self.config.null_response_enabled:
            null_response = NullResponseTool()
            self.tools[null_response.name] = null_response
            log_info("Null response tool enabled")
        
        if self.config.bug_report_enabled:
            bug_report = BugReportTool()
            self.tools[bug_report.name] = bug_report
            log_info("Bug report tool enabled")
        
        if self.config.gpt_image_enabled:
            gpt_image = GPTImageTool()
            self.tools[gpt_image.name] = gpt_image
            log_info("GPT Image tool enabled (gpt-image-1.5)")
        
        if self.config.gemini_image_enabled:
            gemini_image = GeminiImageTool()
            self.tools[gemini_image.name] = gemini_image
            log_info("Gemini Image tool enabled (gemini-3-pro-image-preview)")
        
        if self.config.usage_stats_enabled:
            usage_stats = UsageStatsTool()
            self.tools[usage_stats.name] = usage_stats
            log_info("Usage stats tool enabled")

        if self.config.youtube_search_enabled:
            youtube_search = YouTubeSearchTool()
            self.tools[youtube_search.name] = youtube_search
            log_info("YouTube search tool enabled")
        
        if self.config.source_code_enabled:
            source_code = SourceCodeTool()
            self.tools[source_code.name] = source_code
            log_info("Source code introspection tool enabled")
        
        if self.config.irc_command_enabled:
            irc_command = IRCCommandTool(timeout=self.config.irc_command_timeout)
            self.tools[irc_command.name] = irc_command
            log_info("IRC command tool enabled (permission-based)")
        
        if self.config.claude_code_enabled:
            claude_tech = ClaudeTechTool()
            self.tools[claude_tech.name] = claude_tech
            log_info("Claude tech tool enabled (Opus via Bedrock)")
            
        # Report Status tool is always enabled as it's a core feature for long-running tasks
        report_status = ReportStatusTool()
        self.tools[report_status.name] = report_status
        log_info("Report status tool enabled")
        
        # Knowledge Base tools
        if self.config.kb_learn_enabled:
            kb_learn = KnowledgeBaseLearnTool()
            self.tools[kb_learn.name] = kb_learn
            log_info("Knowledge Base learn tool enabled")
        
        if self.config.kb_search_enabled:
            kb_search = KnowledgeBaseSearchTool()
            self.tools[kb_search.name] = kb_search
            log_info("Knowledge Base search tool enabled")
        
        if self.config.kb_list_enabled:
            kb_list = KnowledgeBaseListTool()
            self.tools[kb_list.name] = kb_list
            log_info("Knowledge Base list tool enabled")
        
        if self.config.kb_forget_enabled:
            kb_forget = KnowledgeBaseForgetTool()
            self.tools[kb_forget.name] = kb_forget
            log_info("Knowledge Base forget tool enabled")
        
        # Moltbook posting
        if self.config.moltbook_post_enabled:
            moltbook_post = MoltbookPostTool()
            self.tools[moltbook_post.name] = moltbook_post
            log_info("Moltbook post tool enabled")
        
        # Reminder tool
        if self.config.reminder_enabled:
            from api.tools.reminder import set_reminder_tool
            reminder = ReminderTool()
            self.tools[reminder.name] = reminder
            set_reminder_tool(reminder)  # Register global instance for join-check endpoint
            log_info("Reminder tool enabled")
    
    def generate_response(self, user_message: str, request_id: str) -> str:
        """
        Generate AI response to user message.
        
        Args:
            user_message: The user's message/question
            request_id: Request ID for logging
            
        Returns:
            AI-generated response text
        """
        log_info(f"[{request_id}] Generating AI response")
        
        try:
            # Build tool definitions
            tool_defs = [tool.get_definition() for tool in self.tools.values()]
            
            # Prepare the input with system prompt
            full_input = f"{self.config.system_prompt}\n\nUser: {user_message}"
            
            # Make API request with extended cache retention
            response = self.client.responses.create(
                model=self.config.model_name,
                input=full_input,
                reasoning={"effort": self.config.reasoning_effort},
                text={"verbosity": self.config.verbosity},
                max_output_tokens=self.config.max_output_tokens,
                tools=tool_defs if tool_defs else None,
                timeout=self.config.timeout,
                prompt_cache_retention="24h"
            )
            
            # Check which tools were used
            self._check_tools_used(response, request_id)
            
            # Extract response text and citations
            output_text = self._extract_output(response, request_id)
            citations = self._extract_citations(response, request_id)
            
            # Clean response for IRC (remove newlines, strip markdown links, append sources)
            cleaned_text = self._clean_for_irc(output_text, citations)
            
            # Validate we have a response
            if not cleaned_text or cleaned_text.strip() == "":
                log_error(f"[{request_id}] AI returned empty response")
                return "I couldn't generate a proper response. Please try again."
            
            log_info(f"[{request_id}] AI response generated successfully")
            return cleaned_text
        
        except Exception as e:
            log_error(f"[{request_id}] Error generating AI response: {e}")
            import traceback
            log_error(f"[{request_id}] Traceback: {traceback.format_exc()}")
            return "Sorry, I encountered an error generating a response."
    
    def generate_response_with_context_stream(
        self, 
        user_message: str, 
        nick: str, 
        channel: str,
        conversation_history: list,
        permission_level: str,
        request_id: str,
        deep_mode: bool = False
    ):
        """
        Generate AI response with conversation context, streaming updates.
        
        Args:
            deep_mode: If True, uses high reasoning effort and deep research instructions
        
        Yields:
            Dict containing 'status' and 'message' keys.
            status: "processing" for intermediate updates, "success" or "error" for final
        """
        log_info(f"[{request_id}] Generating AI response with context (permission: {permission_level})" +
                 (" [DEEP MODE]" if deep_mode else ""))
        
        # Check deep mode rate limit before proceeding
        if deep_mode:
            from api.tools.deep_mode_limit import check_deep_mode_limit, record_deep_mode_usage
            allowed, limit_msg = check_deep_mode_limit(nick, permission_level)
            if not allowed:
                log_warning(f"[{request_id}] Deep mode rate limit reached for {nick}")
                yield {
                    "status": "error",
                    "message": limit_msg
                }
                return
        
        try:
            # Build tool definitions
            tool_defs = [tool.get_definition() for tool in self.tools.values()]
            
            # Build the context-aware prompt
            full_input = self._build_context_prompt(
                user_message, 
                nick, 
                channel, 
                conversation_history,
                deep_mode=deep_mode
            )
            
            # Override settings for deep mode
            reasoning_effort = "high" if deep_mode else self.config.reasoning_effort
            max_tokens = 16000 if deep_mode else self.config.max_output_tokens
            # Deep mode gets 8 minute timeout (vs 4 min normal) for thorough research
            request_timeout = 480 if deep_mode else self.config.timeout
            
            # Make API request with extended cache retention for better prefix caching
            response = self.client.responses.create(
                model=self.config.model_name,
                input=full_input,
                reasoning={"effort": reasoning_effort},
                text={"verbosity": self.config.verbosity},
                max_output_tokens=max_tokens,
                tools=tool_defs if tool_defs else None,
                timeout=request_timeout,
                prompt_cache_retention="24h"
            )
            
            # Check which tools were used
            self._check_tools_used(response, request_id)
            
            # Handle function calls with streaming support
            response_generator = self._handle_function_calls_stream(
                response, full_input, request_id, permission_level, nick, channel, deep_mode
            )
            
            final_response = None
            total_usage = {}
            null_response_triggered = False
            accumulated_citations = []
            
            for event in response_generator:
                if event["type"] == "status_update":
                    # Yield intermediate status update
                    yield {
                        "status": "processing",
                        "message": event["message"]
                    }
                elif event["type"] == "final_result":
                    final_response = event["response"]
                    total_usage = event["usage"]
                    null_response_triggered = event["null_triggered"]
                    accumulated_citations = event.get("citations", [])
            
            # Log usage to database
            log_usage(
                request_id=request_id,
                nick=nick,
                channel=channel,
                model=self.config.model_name,
                input_tokens=total_usage.get("input_tokens", 0),
                cached_tokens=total_usage.get("cached_tokens", 0),
                output_tokens=total_usage.get("output_tokens", 0),
                tool_calls=total_usage.get("tool_calls", 0),
                web_search_calls=total_usage.get("web_search_calls", 0),
                code_interpreter_calls=total_usage.get("code_interpreter_calls", 0)
            )
            
            # Check if null response triggered
            if null_response_triggered:
                log_info(f"[{request_id}] Null response triggered - staying silent")
                yield {
                    "status": "null",
                    "message": ""
                }
                return
            
            # Extract response text
            output_text = self._extract_output(final_response, request_id)
            
            # Use accumulated citations from all iterations, plus any from final response
            final_citations = self._extract_citations(final_response, request_id)
            # Merge: accumulated first, then any new ones from final response
            all_citation_urls = set(accumulated_citations)
            for url in final_citations:
                all_citation_urls.add(url)
            # Preserve order: accumulated first, then new ones
            merged_citations = accumulated_citations.copy()
            for url in final_citations:
                if url not in accumulated_citations:
                    merged_citations.append(url)
            
            # Clean response (strip markdown links, append sources at end)
            cleaned_text = self._clean_for_irc(output_text, merged_citations)
            
            if not cleaned_text or cleaned_text.strip() == "":
                log_error(f"[{request_id}] AI returned empty response")
                yield {
                    "status": "error", 
                    "message": "I couldn't generate a proper response. Please try again."
                }
                return
            
            log_info(f"[{request_id}] AI response generated successfully")
            
            # Record deep mode usage after successful completion
            if deep_mode:
                from api.tools.deep_mode_limit import record_deep_mode_usage
                record_deep_mode_usage(nick, permission_level)
                log_info(f"[{request_id}] Deep mode usage recorded for {nick}")
            
            yield {
                "status": "success",
                "message": cleaned_text
            }
        
        except Exception as e:
            log_error(f"[{request_id}] Error generating AI response: {e}")
            import traceback
            log_error(f"[{request_id}] Traceback: {traceback.format_exc()}")
            yield {
                "status": "error",
                "message": "Sorry, I encountered an error generating a response."
            }

    def generate_response_with_context(
        self, 
        user_message: str, 
        nick: str, 
        channel: str,
        conversation_history: list,
        permission_level: str,
        request_id: str,
        deep_mode: bool = False
    ) -> str:
        """
        Legacy blocking method for backward compatibility.
        Wraps the streaming method and just returns the final result.
        """
        generator = self.generate_response_with_context_stream(
            user_message, nick, channel, conversation_history, permission_level, request_id, deep_mode
        )
        
        final_message = "I couldn't generate a proper response."
        
        for event in generator:
            if event["status"] == "success":
                final_message = event["message"]
            elif event["status"] == "null":
                return NULL_RESPONSE_MARKER
                
        return final_message
    
    def _handle_function_calls(self, *args, **kwargs):
        """Legacy wrapper for streaming handler."""
        # Convert generator to final result
        generator = self._handle_function_calls_stream(*args, **kwargs)
        final_response = None
        total_usage = {} 
        null_triggered = False
        
        for event in generator:
            if event["type"] == "final_result":
                final_response = event["response"]
                total_usage = event["usage"]
                null_triggered = event["null_triggered"]
        
        return final_response, null_triggered, total_usage

    def _handle_function_calls_stream(self, response: Any, original_input: str, request_id: str, permission_level: str = "normal", nick: str = "", channel: str = "", deep_mode: bool = False):
        """
        Handle function calls in the response using multi-turn tool calling.
        Yields status events during execution.
        """
        import json
        
        # Deep mode gets more iterations for thorough research (30 vs 18)
        MAX_TOOL_ITERATIONS = 30 if deep_mode else 18
        iteration = 0
        null_response_triggered = False
        
        # Track all citations across iterations
        all_citations = []
        seen_citation_urls = set()
        
        # Track cumulative usage across all iterations
        total_usage = {
            "input_tokens": 0,
            "cached_tokens": 0,
            "output_tokens": 0,
            "tool_calls": 0,
            "web_search_calls": 0,
            "code_interpreter_calls": 0
        }
        
        # Helper to count tool types from output items
        def count_tools_in_output(output_items):
            counts = {"function": 0, "web_search": 0, "code_interpreter": 0}
            if output_items:
                for item in output_items:
                    item_type = getattr(item, 'type', None)
                    if item_type == 'function_call':
                        counts["function"] += 1
                    elif item_type == 'web_search_call':
                        counts["web_search"] += 1
                    elif item_type == 'code_interpreter_call':
                        counts["code_interpreter"] += 1
            return counts
        
        # Extract usage from initial response
        initial_usage = extract_usage_from_response(response)
        total_usage["input_tokens"] += initial_usage.get("input_tokens", 0)
        total_usage["cached_tokens"] += initial_usage.get("cached_tokens", 0)
        total_usage["output_tokens"] += initial_usage.get("output_tokens", 0)
        
        # Count initial response's tool calls (this was the bug - we weren't counting these)
        initial_output = getattr(response, 'output', None)
        initial_counts = count_tools_in_output(initial_output)
        total_usage["tool_calls"] += initial_counts["function"]
        total_usage["web_search_calls"] += initial_counts["web_search"]
        total_usage["code_interpreter_calls"] += initial_counts["code_interpreter"]
        
        while iteration < MAX_TOOL_ITERATIONS:
            iteration += 1
            
            # Collect citations from current response
            iter_citations = self._extract_citations(response, request_id)
            for url in iter_citations:
                if url not in seen_citation_urls:
                    seen_citation_urls.add(url)
                    all_citations.append(url)
            
            output_items = getattr(response, 'output', None)
            if not output_items:
                log_debug(f"[{request_id}] No output items in response")
                yield {
                    "type": "final_result",
                    "response": response,
                    "null_triggered": null_response_triggered,
                    "usage": total_usage,
                    "citations": all_citations
                }
                return
            
            # Check for function calls
            function_calls = []
            for item in output_items:
                item_type = getattr(item, 'type', None)
                if item_type == 'function_call':
                    function_calls.append(item)
            
            if not function_calls:
                # No more function calls, we're done
                yield {
                    "type": "final_result",
                    "response": response,
                    "null_triggered": null_response_triggered,
                    "usage": total_usage,
                    "citations": all_citations
                }
                return
            
            log_info(f"[{request_id}] Iteration {iteration}: Executing {len(function_calls)} function call(s)")
            
            # Get response ID for chaining
            response_id = getattr(response, 'id', None)
            
            # Collect function outputs for this iteration
            function_outputs = []
            
            for func_call in function_calls:
                func_name = getattr(func_call, 'name', None)
                func_args_raw = getattr(func_call, 'arguments', {})
                call_id = getattr(func_call, 'call_id', None)
                
                # Parse arguments if they're a JSON string
                func_args = {}
                if isinstance(func_args_raw, str):
                    try:
                        func_args = json.loads(func_args_raw)
                    except json.JSONDecodeError as e:
                        log_warning(f"[{request_id}] Failed to parse tool arguments for {func_name}: {e}")
                        function_outputs.append({
                            "type": "function_call_output",
                            "call_id": call_id,
                            "output": f"Error: Invalid JSON in tool arguments - {e}"
                        })
                        continue
                else:
                    func_args = func_args_raw
                
                if func_name not in self.tools:
                    function_outputs.append({
                        "type": "function_call_output",
                        "call_id": call_id,
                        "output": f"Error: Unknown tool '{func_name}'"
                    })
                    continue
                
                tool = self.tools[func_name]
                log_info(f"[{request_id}] Executing {func_name} with args: {func_args}")
                
                try:
                    # Check image rate limit for image tools
                    if is_image_tool(func_name):
                        allowed, rate_limit_msg = check_image_rate_limit(permission_level)
                        if not allowed:
                            log_warning(f"[{request_id}] Image rate limit reached for {func_name}")
                            function_outputs.append({
                                "type": "function_call_output",
                                "call_id": call_id,
                                "output": rate_limit_msg
                            })
                            continue
                    
                    # Inject permission_level/context for specific tools
                    if func_name in ('manage_user_rules', 'execute_shell', 'bug_report', 'irc_command', 'reminder'):
                        func_args['permission_level'] = permission_level
                        if func_name == 'bug_report':
                            func_args['requesting_user'] = nick
                            func_args['channel'] = channel
                        if func_name == 'reminder':
                            func_args['requesting_user'] = nick
                            func_args['channel'] = channel
                    
                    # Inject current channel and permission for chat history access control
                    if func_name == 'query_chat_history':
                        func_args['_current_channel'] = channel
                        func_args['_permission_level'] = permission_level
                    
                    # Execute the tool
                    result = tool.execute(**func_args)
                    
                    # Record successful image generation for rate limiting
                    if is_image_tool(func_name) and not result.startswith("Error"):
                        record_image_generation()
                    
                    # Check for status updates
                    if func_name == 'report_status' and result.startswith(STATUS_UPDATE_MARKER):
                        status_msg = result.replace(STATUS_UPDATE_MARKER, "")
                        log_success(f"[{request_id}] Status update: {status_msg}")
                        yield {
                            "type": "status_update",
                            "message": status_msg
                        }
                        # We return a generic success message to the LLM so it continues
                        result = "Status reported to user."
                    else:
                        log_success(f"[{request_id}] {func_name} executed successfully")
                    
                    # Check for null response marker
                    if result == NULL_RESPONSE_MARKER:
                        null_response_triggered = True
                    
                    # Handle image analysis special case (recurses)
                    if func_name == 'analyze_image':
                        try:
                            result_data = json.loads(result)
                            if result_data.get('status') == 'success' and 'image_data' in result_data:
                                log_info(f"[{request_id}] Running vision analysis on image...")
                                yield {
                                    "type": "status_update",
                                    "message": "Analyzing image content..."
                                }
                                
                                image_data = result_data['image_data']
                                detail = image_data.get('detail', 'auto')
                                question = result_data.get('question', 'Describe this image.')
                                
                                # Make separate vision call
                                vision_response = self.client.responses.create(
                                    model=self.config.model_name,
                                    input=[{
                                        "type": "message",
                                        "role": "user",
                                        "content": [
                                            {
                                                "type": "input_image",
                                                "image_url": image_data['image_url']
                                            },
                                            {
                                                "type": "input_text",
                                                "text": f"Please analyze this image. {question}"
                                            }
                                        ]
                                    }],
                                    max_output_tokens=1000,
                                    timeout=60
                                )
                                
                                # Extract description
                                analysis = self._extract_output(vision_response, f"{request_id}_vision")
                                
                                # Replace the huge result with the text description
                                result = f"Image Analysis Result:\n{analysis}"
                                log_success(f"[{request_id}] Vision analysis complete")
                                
                        except Exception as e:
                            log_error(f"[{request_id}] Vision analysis failed: {e}")
                            result = f"Error analyzing image: {str(e)}"

                    # Standard function output
                    function_outputs.append({
                        "type": "function_call_output",
                        "call_id": call_id,
                        "output": result
                    })
                    
                except Exception as e:
                    log_error(f"[{request_id}] Error executing {func_name}: {e}")
                    function_outputs.append({
                        "type": "function_call_output",
                        "call_id": call_id,
                        "output": f"Error executing tool: {str(e)}"
                    })
            
            # If we have function outputs, make the next API call
            if function_outputs:
                tool_defs = [t.get_definition() for t in self.tools.values()]
                
                # Use previous_response_id for multi-turn - this enables CoT passing
                # and better caching as documented in GPT-5.2 guide
                response = self.client.responses.create(
                    model=self.config.model_name,
                    input=function_outputs,
                    previous_response_id=response_id,
                    tools=tool_defs if tool_defs else None,
                    reasoning={"effort": self.config.reasoning_effort},
                    text={"verbosity": self.config.verbosity},
                    max_output_tokens=self.config.max_output_tokens,
                    timeout=self.config.timeout,
                    prompt_cache_retention="24h"
                )
                
                # Track usage
                iter_usage = extract_usage_from_response(response)
                total_usage["input_tokens"] += iter_usage.get("input_tokens", 0)
                total_usage["cached_tokens"] += iter_usage.get("cached_tokens", 0)
                total_usage["output_tokens"] += iter_usage.get("output_tokens", 0)
                
                # Count tools from the NEW response (not the previous one we just processed)
                new_output = getattr(response, 'output', None)
                new_counts = count_tools_in_output(new_output)
                total_usage["tool_calls"] += new_counts["function"]
                total_usage["web_search_calls"] += new_counts["web_search"]
                total_usage["code_interpreter_calls"] += new_counts["code_interpreter"]
                
                self._check_tools_used(response, request_id)
            else:
                yield {
                    "type": "final_result",
                    "response": response,
                    "null_triggered": null_response_triggered,
                    "usage": total_usage,
                    "citations": all_citations
                }
                return
        
        log_warning(f"[{request_id}] Reached max tool iterations ({MAX_TOOL_ITERATIONS})")
        yield {
            "type": "final_result",
            "response": response,
            "null_triggered": null_response_triggered,
            "usage": total_usage,
            "citations": all_citations
        }
    
    def _build_context_prompt(
        self, 
        user_message: str, 
        nick: str, 
        channel: str,
        conversation_history: list,
        deep_mode: bool = False
    ) -> str:
        """
        Build a prompt with conversation context.
        
        IMPORTANT: Structure is optimized for OpenAI prompt caching.
        The system prompt and user rules form a STABLE PREFIX that gets cached.
        The current question and conversation history come AFTER so they can
        change without invalidating the cached prefix.
        
        Args:
            user_message: The current mention message
            nick: User who mentioned the bot
            channel: Channel where mention occurred
            conversation_history: List of recent messages
            deep_mode: If True, inject deep research instructions
            
        Returns:
            Formatted prompt with context
        """
        # System prompt is static (no datetime injection) for better caching
        prompt_parts = [self.config.system_prompt, ""]
        
        # Inject deep research instructions if deep_mode is enabled
        if deep_mode:
            deep_instructions = """=== DEEP RESEARCH MODE ACTIVATED ===
You are in DEEP RESEARCH MODE. This means the user wants a thorough, well-researched answer.

**PROGRESS UPDATES (REQUIRED):**
Use the report_status tool to announce what you're doing at each major step:
- "Searching for information on [topic]..."
- "Found relevant sources, analyzing..."
- "Researching [specific aspect]..."
- "Compiling findings into comprehensive answer..."
This keeps the user informed during the longer research process.

**THOROUGH RESEARCH:**
Perform AT LEAST 2-3 web searches on different aspects of the topic:
- Search for the main topic/question
- Search for related concepts, alternatives, or comparisons  
- Search for recent developments or expert opinions
- Use fetch_url to read full articles when snippets aren't enough

**USE ALL AVAILABLE TOOLS:**
You have access to ALL tools in deep mode - use them as needed:
- Web search for current information
- Python execution for calculations, data analysis, code examples
- Image analysis if images are involved
- fetch_url to read full web pages
- Any other tool that helps answer the question thoroughly

**HIGH QUALITY OUTPUT:**
- Include multiple perspectives where relevant
- Cite sources and provide links
- Structure with headers and sections
- Include examples, explanations, and context

**USE PASTE TOOL FOR FINAL ANSWER:**
Your response will likely be long. Use the paste tool to create a formatted document 
with your full answer. Return only the paste URL to IRC with a brief summary.

Remember: Quality over speed. The user specifically requested deep research with --deep flag.
=== END DEEP RESEARCH MODE ===
"""
            prompt_parts.append(deep_instructions)
            prompt_parts.append("")
        
        # Inject user-specific rules if they exist and are enabled
        # Note: User rules are semi-stable (change rarely) so they're part of the prefix
        user_rules = self._get_user_rules(nick)
        if user_rules:
            prompt_parts.append("=== CUSTOM RULES FOR THIS USER ===")
            prompt_parts.append(f"The following custom rules have been set by/for {nick}. Apply these rules when responding to them:")
            prompt_parts.append(user_rules)
            prompt_parts.append("=== END CUSTOM RULES ===")
            prompt_parts.append("")
        
        # Add the current question BEFORE history (cache optimization)
        # This way the system prompt prefix stays stable and cacheable
        # Include timestamp so model knows current time without it being in system prompt
        from datetime import datetime
        current_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        
        prompt_parts.append("=== CURRENT QUESTION ===")
        prompt_parts.append(f"Timestamp: {current_time}")
        prompt_parts.append(f"Channel: {channel}")
        prompt_parts.append(f"User: {nick}")
        prompt_parts.append(f"Message: {user_message}")
        prompt_parts.append("")
        
        # Add conversation history AFTER the question (at the end)
        # Changes to history won't invalidate the cached system prompt prefix
        if conversation_history and len(conversation_history) > 0:
            prompt_parts.append("=== RECENT CONVERSATION CONTEXT ===")
            prompt_parts.append(f"(Last {len(conversation_history)} messages from {channel} for context)")
            prompt_parts.append("")
            
            for msg in conversation_history:
                # Format: [timestamp] nickname: message
                prompt_parts.append(f"[{msg.timestamp}] {msg.nick}: {msg.content}")
            
            prompt_parts.append("")
            prompt_parts.append("=== END OF CONTEXT ===")
        
        prompt_parts.append("")
        prompt_parts.append("Please respond to the CURRENT QUESTION above. Use the conversation context if relevant, but focus on answering what was just asked.")
        
        return "\n".join(prompt_parts)
    
    def _extract_citations(self, response: Any, request_id: str) -> List[str]:
        """
        Extract all URL citations from API response annotations.
        
        Args:
            response: OpenAI API response
            request_id: Request ID for logging
            
        Returns:
            List of unique cleaned URLs from citations
        """
        urls = []
        seen_urls = set()
        
        try:
            if hasattr(response, 'output'):
                for item in response.output:
                    if hasattr(item, 'type') and item.type == 'message':
                        if hasattr(item, 'content') and item.content:
                            for content_item in item.content:
                                # Check for annotations in the content item
                                annotations = getattr(content_item, 'annotations', None)
                                if annotations:
                                    for annotation in annotations:
                                        ann_type = getattr(annotation, 'type', None)
                                        if ann_type == 'url_citation':
                                            url = getattr(annotation, 'url', None)
                                            if url:
                                                # Clean the URL
                                                clean_url = self._clean_citation_url(url)
                                                if clean_url and clean_url not in seen_urls:
                                                    seen_urls.add(clean_url)
                                                    urls.append(clean_url)
        except Exception as e:
            log_debug(f"[{request_id}] Error extracting citations: {e}")
        
        return urls
    
    def _clean_citation_url(self, url: str) -> str:
        """
        Clean a citation URL by removing tracking parameters.
        
        Args:
            url: Raw URL from citation
            
        Returns:
            Cleaned URL without tracking params
        """
        if not url:
            return url
        
        import re
        from urllib.parse import urlparse, urlunparse, parse_qs, urlencode
        
        try:
            parsed = urlparse(url)
            
            # Parse query params and remove tracking ones
            if parsed.query:
                params = parse_qs(parsed.query, keep_blank_values=True)
                # Remove common tracking parameters
                tracking_params = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content']
                for param in tracking_params:
                    params.pop(param, None)
                
                # Rebuild query string
                new_query = urlencode(params, doseq=True) if params else ''
                parsed = parsed._replace(query=new_query)
            
            return urlunparse(parsed)
        except Exception:
            # If parsing fails, just do simple string replacement
            return re.sub(r'\?utm_source=openai(&|$)', '', url)
    
    def _extract_output(self, response: Any, request_id: str) -> str:
        """
        Extract output text from API response.
        
        Args:
            response: OpenAI API response
            request_id: Request ID for logging
            
        Returns:
            Extracted text
        """
        try:
            # Get the main output text (this is the standard way)
            if hasattr(response, 'output_text'):
                output = response.output_text
                if output:
                    return output
                else:
                    log_warning(f"[{request_id}] output_text is empty")
            
            # Fallback: try to extract from output items
            if hasattr(response, 'output'):
                for item in response.output:
                    if hasattr(item, 'type') and item.type == 'message':
                        if hasattr(item, 'content') and item.content:
                            for content_item in item.content:
                                if hasattr(content_item, 'type') and content_item.type == 'output_text':
                                    text = content_item.text
                                    if text:
                                        return text
            
            # Also try output_items for older SDK versions
            if hasattr(response, 'output_items'):
                for item in response.output_items:
                    if hasattr(item, 'type') and item.type == 'message':
                        if hasattr(item, 'content') and item.content:
                            for content_item in item.content:
                                if hasattr(content_item, 'text'):
                                    text = content_item.text
                                    if text:
                                        return text
            
            # Debug: log the response structure
            log_error(f"[{request_id}] No output found in response, response type: {type(response)}")
            if hasattr(response, 'output'):
                log_error(f"[{request_id}] Response has 'output' with {len(response.output)} items")
                for i, item in enumerate(response.output):
                    log_error(f"[{request_id}] Output item {i}: type={getattr(item, 'type', 'unknown')}")
            return "No response generated"
        
        except Exception as e:
            log_error(f"[{request_id}] Error extracting output: {e}")
            import traceback
            log_error(f"[{request_id}] Traceback: {traceback.format_exc()}")
            return "Error processing response"
    
    def _get_user_rules(self, nick: str) -> Optional[str]:
        """
        Get active custom rules for a user.
        
        Args:
            nick: User's nickname
            
        Returns:
            Rules text if rules exist and are enabled, None otherwise
        """
        # Check if user rules tool is enabled and available
        if "manage_user_rules" in self.tools:
            user_rules_tool = self.tools["manage_user_rules"]
            return user_rules_tool.get_active_rules(nick)
        return None
    
    def _clean_for_irc(self, text: str, citations: List[str] = None) -> str:
        """
        Clean text for IRC compatibility.
        
        Removes newlines, excessive whitespace, and inline markdown links.
        Appends collected citations as plain URLs at the end.
        
        Args:
            text: Raw text to clean
            citations: Optional list of citation URLs to append at end
            
        Returns:
            Cleaned text suitable for IRC
        """
        import re
        
        if not text:
            return "I couldn't generate a response."
        
        # Strip inline markdown links [text](url) -> text
        # This regex matches [any text](any url)
        text = re.sub(r'\[([^\]]+)\]\([^)]+\)', r'\1', text)
        
        # Remove leftover parenthetical domain references like (domain.com) or (domain.org)
        # These are artifacts from stripped markdown links
        text = re.sub(r'\s*\([\w.-]+\.(com|org|net|gov|edu|io|co|uk|de|fr|info|dev)\)', '', text)
        
        # Remove model's own "Sources:" section if present (we'll add our own clean one)
        # Match "Sources:" followed by domain names, URLs, or descriptive text until end or period
        text = re.sub(r'\s*Sources?:\s*[^|]*?(?=\s*\||$)', '', text, flags=re.IGNORECASE)
        
        # Replace newlines with spaces
        text = text.replace('\n', ' ').replace('\r', ' ')
        
        # Replace multiple spaces with single space
        while '  ' in text:
            text = text.replace('  ', ' ')
        
        # Strip leading/trailing whitespace
        text = text.strip()
        
        # Remove trailing periods that might be left over
        text = text.rstrip('.')
        
        # Ensure we have something to return
        if not text:
            return "I couldn't generate a response."
        
        # Append citations at the end if we have any
        if citations and len(citations) > 0:
            sources_text = " | Sources: " + " , ".join(citations)
            text = text + sources_text
        
        return text
