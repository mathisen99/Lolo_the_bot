#!/usr/bin/env python3
"""
Test script for AI mention handler.

Tests the AI client and tools without requiring the full bot setup.
"""

import os
import sys
from pathlib import Path

# Add parent directory to path
sys.path.insert(0, str(Path(__file__).parent.parent))

from api.ai import AIClient, AIConfig
from api.utils.output import log_info, log_success, log_error, log_warning


def test_config():
    """Test configuration loading."""
    log_info("Testing configuration loading...")
    try:
        config = AIConfig()
        log_success(f"✓ Config loaded: model={config.model_name}")
        log_success(f"✓ Reasoning effort: {config.reasoning_effort}")
        log_success(f"✓ Verbosity: {config.verbosity}")
        log_success(f"✓ Max tokens: {config.max_output_tokens}")
        log_success(f"✓ Tools enabled: {', '.join(config.get_enabled_tools())}")
        return config
    except Exception as e:
        log_error(f"✗ Config loading failed: {e}")
        return None


def test_ai_client(config):
    """Test AI client initialization."""
    log_info("Testing AI client initialization...")
    try:
        client = AIClient(config)
        log_success("✓ AI client initialized")
        log_success(f"✓ Tools registered: {', '.join(client.tools.keys())}")
        return client
    except Exception as e:
        log_error(f"✗ AI client initialization failed: {e}")
        return None


def test_simple_response(client):
    """Test simple AI response."""
    log_info("Testing simple response...")
    try:
        response = client.generate_response(
            user_message="What is 2+2?",
            request_id="test-001"
        )
        log_success(f"✓ Response: {response}")
        
        # Check for newlines (should be removed)
        if '\n' in response:
            log_warning("⚠ Response contains newlines (should be cleaned)")
        else:
            log_success("✓ Response properly cleaned for IRC")
        
        return True
    except Exception as e:
        log_error(f"✗ Simple response failed: {e}")
        return False


def test_web_search(client):
    """Test web search capability."""
    if 'web_search' not in client.tools:
        log_warning("⚠ Web search tool not enabled, skipping test")
        return True
    
    log_info("Testing web search...")
    try:
        response = client.generate_response(
            user_message="What's a recent news headline?",
            request_id="test-002"
        )
        log_success(f"✓ Web search response: {response}")
        return True
    except Exception as e:
        log_error(f"✗ Web search failed: {e}")
        return False


def test_python_exec(client):
    """Test Python execution capability."""
    if 'python_exec' not in client.tools:
        log_warning("⚠ Python execution tool not enabled, skipping test")
        return True
    
    log_info("Testing Python execution...")
    try:
        response = client.generate_response(
            user_message="Calculate the factorial of 5 using Python",
            request_id="test-003"
        )
        log_success(f"✓ Python execution response: {response}")
        return True
    except Exception as e:
        log_error(f"✗ Python execution failed: {e}")
        return False


def main():
    """Run all tests."""
    print("\n" + "="*60)
    print("AI Mention Handler Test Suite")
    print("="*60 + "\n")
    
    # Check API key
    if not os.getenv("OPENAI_API_KEY"):
        log_error("✗ OPENAI_API_KEY environment variable not set")
        log_info("Set it with: export OPENAI_API_KEY='sk-your-key-here'")
        return 1
    
    log_success("✓ OPENAI_API_KEY is set")
    print()
    
    # Test configuration
    config = test_config()
    if not config:
        return 1
    print()
    
    # Test AI client
    client = test_ai_client(config)
    if not client:
        return 1
    print()
    
    # Test simple response
    if not test_simple_response(client):
        return 1
    print()
    
    # Test web search (optional)
    test_web_search(client)
    print()
    
    # Test Python execution (optional)
    test_python_exec(client)
    print()
    
    print("="*60)
    log_success("All tests completed!")
    print("="*60 + "\n")
    
    log_info("Next steps:")
    log_info("1. Start the API: uvicorn api.main:app --host 0.0.0.0 --port 8000")
    log_info("2. Start the bot: ./lolo")
    log_info("3. Mention the bot in IRC to test live")
    
    return 0


if __name__ == "__main__":
    sys.exit(main())
