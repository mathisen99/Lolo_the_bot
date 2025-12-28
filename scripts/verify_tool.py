
import os
import sys
from pathlib import Path
from dotenv import load_dotenv

# Load environment variables
load_dotenv()

# Add project root to path so we can import api.tools
project_root = Path(__file__).parent.parent
sys.path.append(str(project_root))

from api.tools.chat_history import ChatHistoryTool

def verify_tool():
    print("Initializing ChatHistoryTool...")
    tool = ChatHistoryTool()
    
    print("\n--- Test 1: Semantic Search (Cats) ---")
    result = tool.execute(
        channel="##llm-bots",
        search_term="what did mathisen say about cats?",
        semantic=True,
        nick="Mathisen"
    )
    print(result)
    
    print("\n--- Test 2: Semantic Search (Invalid Search) ---")
    result = tool.execute(
        channel="##llm-bots",
        search_term="supercalifragilisticexpialidocious_nonsense_term",
        semantic=True
    )
    print(result)

    print("\n--- Test 3: Normal Search (Legacy) ---")
    result = tool.execute(
        channel="##llm-bots",
        search_term="cat",
        limit=2
    )
    print(result)

if __name__ == "__main__":
    verify_tool()
