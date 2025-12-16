"""
AI client for GPT-5.1 integration.

Handles communication with OpenAI API and tool execution.
"""

from typing import List, Dict, Any, Optional
from openai import OpenAI
from .config import AIConfig
from api.tools import WebSearchTool, PythonExecTool, FluxCreateTool, FluxEditTool, ImageAnalysisTool, FetchUrlTool, UserRulesTool, ChatHistoryTool, PasteTool, ShellExecTool, VoiceSpeakTool, NullResponseTool, NULL_RESPONSE_MARKER, BugReportTool, GPTImageTool
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
            log_info("Python execution tool enabled (code_interpreter)")
        
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
            
            # Make API request
            response = self.client.responses.create(
                model=self.config.model_name,
                input=full_input,
                reasoning={"effort": self.config.reasoning_effort},
                text={"verbosity": self.config.verbosity},
                max_output_tokens=self.config.max_output_tokens,
                tools=tool_defs if tool_defs else None,
                timeout=self.config.timeout
            )
            
            # Check which tools were used
            self._check_tools_used(response, request_id)
            
            # Extract response text
            output_text = self._extract_output(response, request_id)
            
            # Clean response for IRC (remove newlines)
            cleaned_text = self._clean_for_irc(output_text)
            
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
    
    def generate_response_with_context(
        self, 
        user_message: str, 
        nick: str, 
        channel: str,
        conversation_history: list,
        permission_level: str,
        request_id: str
    ) -> str:
        """
        Generate AI response with conversation context.
        
        Args:
            user_message: The current mention message
            nick: User who mentioned the bot
            channel: Channel where mention occurred
            conversation_history: List of recent messages for context
            permission_level: User's permission level (owner, admin, normal, ignored)
            request_id: Request ID for logging
            
        Returns:
            AI-generated response text
        """
        log_info(f"[{request_id}] Generating AI response with context (permission: {permission_level})")
        
        try:
            # Build tool definitions
            tool_defs = [tool.get_definition() for tool in self.tools.values()]
            
            # Build the context-aware prompt
            full_input = self._build_context_prompt(
                user_message, 
                nick, 
                channel, 
                conversation_history
            )
            
            # Make API request
            response = self.client.responses.create(
                model=self.config.model_name,
                input=full_input,
                reasoning={"effort": self.config.reasoning_effort},
                text={"verbosity": self.config.verbosity},
                max_output_tokens=self.config.max_output_tokens,
                tools=tool_defs if tool_defs else None,
                timeout=self.config.timeout
            )
            
            # Check which tools were used
            self._check_tools_used(response, request_id)
            
            # Handle function calls if present
            response, null_response_triggered = self._handle_function_calls(response, full_input, request_id, permission_level, nick, channel)
            
            # Check if null response was triggered (user asked for silence)
            # Return special marker that tells Go bot to stay silent
            if null_response_triggered:
                log_info(f"[{request_id}] Null response triggered - staying silent")
                return NULL_RESPONSE_MARKER
            
            # Extract response text
            output_text = self._extract_output(response, request_id)
            
            # Clean response for IRC (remove newlines)
            cleaned_text = self._clean_for_irc(output_text)
            
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
    
    def _handle_function_calls(self, response: Any, original_input: str, request_id: str, permission_level: str = "normal", nick: str = "", channel: str = "") -> tuple[Any, bool]:
        """
        Handle function calls in the response using multi-turn tool calling.
        
        Uses previous_response_id to maintain reasoning chain across turns.
        Loops until the model stops requesting function calls or max iterations reached.
        All tools are treated uniformly - outputs feed back into the loop for potential chaining.
        
        Args:
            response: Initial API response
            original_input: Original input prompt
            request_id: Request ID for logging
            permission_level: User's permission level for tool authorization
            nick: User's nickname for context
            channel: Channel name for context
            
        Returns:
            Tuple of (final response, null_response_triggered)
        """
        import json
        
        MAX_TOOL_ITERATIONS = 10  # Safety limit to prevent infinite loops
        iteration = 0
        null_response_triggered = False
        
        while iteration < MAX_TOOL_ITERATIONS:
            iteration += 1
            
            output_items = getattr(response, 'output', None)
            if not output_items:
                log_debug(f"[{request_id}] No output items in response")
                return response, null_response_triggered
            
            # Check for function calls
            function_calls = []
            for item in output_items:
                item_type = getattr(item, 'type', None)
                if item_type == 'function_call':
                    function_calls.append(item)
            
            if not function_calls:
                # No more function calls, we're done
                return response, null_response_triggered
            
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
                if isinstance(func_args_raw, str):
                    func_args = json.loads(func_args_raw)
                else:
                    func_args = func_args_raw
                
                if func_name not in self.tools:
                    log_warning(f"[{request_id}] Unknown tool: {func_name}")
                    function_outputs.append({
                        "type": "function_call_output",
                        "call_id": call_id,
                        "output": f"Error: Unknown tool '{func_name}'"
                    })
                    continue
                
                tool = self.tools[func_name]
                log_info(f"[{request_id}] Executing {func_name} with args: {func_args}")
                
                try:
                    # Inject permission_level for tools that need it
                    if func_name in ('manage_user_rules', 'execute_shell'):
                        func_args['permission_level'] = permission_level
                    
                    # Inject full context for bug_report tool
                    if func_name == 'bug_report':
                        func_args['permission_level'] = permission_level
                        func_args['requesting_user'] = nick
                        func_args['channel'] = channel
                    
                    # Execute the tool
                    result = tool.execute(**func_args)
                    log_success(f"[{request_id}] {func_name} executed successfully: {result[:100] if len(result) > 100 else result}")
                    
                    # Check for null response marker
                    if result == NULL_RESPONSE_MARKER:
                        null_response_triggered = True
                        log_info(f"[{request_id}] Null response marker detected - will stay silent")
                    
                    # Handle image analysis - needs vision API call but continues the chain
                    if func_name == 'analyze_image':
                        result_data = json.loads(result)
                        
                        if result_data.get('status') == 'error':
                            function_outputs.append({
                                "type": "function_call_output",
                                "call_id": call_id,
                                "output": f"Error: {result_data.get('error', 'Unknown error')}"
                            })
                        else:
                            # Make vision API call to analyze the image
                            image_data = result_data.get('image_data', {})
                            question = result_data.get('question', '')
                            
                            question_text = f" Question: {question}" if question else ""
                            vision_prompt = f"Analyze this image in detail.{question_text} Provide a comprehensive description that could be used to recreate or understand the image."
                            
                            vision_response = self.client.responses.create(
                                model=self.config.model_name,
                                input=[
                                    {
                                        "role": "user",
                                        "content": [
                                            {"type": "input_text", "text": vision_prompt},
                                            image_data
                                        ]
                                    }
                                ],
                                reasoning={"effort": "medium"},
                                text={"verbosity": self.config.verbosity},
                                max_output_tokens=5000,
                                timeout=self.config.timeout
                            )
                            
                            # Extract the analysis text and feed it back as tool output
                            analysis_text = self._extract_output(vision_response, request_id)
                            log_info(f"[{request_id}] Image analysis complete, feeding back to chain")
                            
                            function_outputs.append({
                                "type": "function_call_output",
                                "call_id": call_id,
                                "output": analysis_text
                            })
                    else:
                        # Standard function output - all tools treated the same
                        function_outputs.append({
                            "type": "function_call_output",
                            "call_id": call_id,
                            "output": result
                        })
                    
                except Exception as e:
                    log_error(f"[{request_id}] Error executing {func_name}: {e}")
                    import traceback
                    log_error(f"[{request_id}] Traceback: {traceback.format_exc()}")
                    function_outputs.append({
                        "type": "function_call_output",
                        "call_id": call_id,
                        "output": f"Error executing tool: {str(e)}"
                    })
            
            # If we have function outputs, make the next API call
            if function_outputs:
                tool_defs = [t.get_definition() for t in self.tools.values()]
                
                # Use previous_response_id to maintain reasoning chain
                # This is the recommended approach from GPT-5.1 docs
                response = self.client.responses.create(
                    model=self.config.model_name,
                    input=function_outputs,
                    previous_response_id=response_id,
                    tools=tool_defs if tool_defs else None,
                    reasoning={"effort": self.config.reasoning_effort},
                    text={"verbosity": self.config.verbosity},
                    max_output_tokens=self.config.max_output_tokens,
                    timeout=self.config.timeout
                )
                
                # Check which tools were used in this iteration
                self._check_tools_used(response, request_id)
            else:
                # No outputs to send, we're done
                return response, null_response_triggered
        
        log_warning(f"[{request_id}] Reached max tool iterations ({MAX_TOOL_ITERATIONS})")
        return response, null_response_triggered
    
    def _build_context_prompt(
        self, 
        user_message: str, 
        nick: str, 
        channel: str,
        conversation_history: list
    ) -> str:
        """
        Build a prompt with conversation context.
        
        Args:
            user_message: The current mention message
            nick: User who mentioned the bot
            channel: Channel where mention occurred
            conversation_history: List of recent messages
            
        Returns:
            Formatted prompt with context
        """
        from datetime import datetime
        
        # Get current date/time and inject into system prompt
        current_datetime = datetime.now().strftime("%A, %B %d, %Y at %H:%M:%S UTC")
        system_prompt_with_time = self.config.system_prompt.format(current_datetime=current_datetime)
        
        prompt_parts = [system_prompt_with_time, ""]
        
        # Inject user-specific rules if they exist and are enabled
        user_rules = self._get_user_rules(nick)
        if user_rules:
            prompt_parts.append("=== CUSTOM RULES FOR THIS USER ===")
            prompt_parts.append(f"The following custom rules have been set by/for {nick}. Apply these rules when responding to them:")
            prompt_parts.append(user_rules)
            prompt_parts.append("=== END CUSTOM RULES ===")
            prompt_parts.append("")
        
        # Add conversation history if available
        if conversation_history and len(conversation_history) > 0:
            prompt_parts.append("=== RECENT CONVERSATION HISTORY ===")
            prompt_parts.append(f"(Last {len(conversation_history)} messages from {channel})")
            prompt_parts.append("")
            
            for msg in conversation_history:
                # Format: [timestamp] nickname: message
                prompt_parts.append(f"[{msg.timestamp}] {msg.nick}: {msg.content}")
            
            prompt_parts.append("")
            prompt_parts.append("=== END OF HISTORY ===")
            prompt_parts.append("")
        
        # Add the current question clearly separated
        prompt_parts.append("=== CURRENT QUESTION ===")
        prompt_parts.append(f"Channel: {channel}")
        prompt_parts.append(f"User: {nick}")
        prompt_parts.append(f"Message: {user_message}")
        prompt_parts.append("")
        prompt_parts.append("Please respond to the CURRENT QUESTION above. Use the conversation history for context if relevant, but focus on answering what was just asked.")
        
        return "\n".join(prompt_parts)
    
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
    
    def _clean_for_irc(self, text: str) -> str:
        """
        Clean text for IRC compatibility.
        
        Removes newlines and excessive whitespace to ensure
        the message works well with IRC message splitting.
        
        Args:
            text: Raw text to clean
            
        Returns:
            Cleaned text suitable for IRC
        """
        if not text:
            return "I couldn't generate a response."
        
        # Replace newlines with spaces
        text = text.replace('\n', ' ').replace('\r', ' ')
        
        # Replace multiple spaces with single space
        while '  ' in text:
            text = text.replace('  ', ' ')
        
        # Strip leading/trailing whitespace
        text = text.strip()
        
        # Ensure we have something to return
        if not text:
            return "I couldn't generate a response."
        
        return text
