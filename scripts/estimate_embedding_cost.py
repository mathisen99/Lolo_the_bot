
import sqlite3
import tiktoken
import os
from pathlib import Path
from dotenv import load_dotenv

# Load environment variables
load_dotenv()

def estimate_cost():
    # Configuration
    DB_PATH = Path("data/bot.db")
    MODEL_PRICE_PER_1M_TOKENS = 0.02  # text-embedding-3-small pricing
    ENCODING_NAME = "cl100k_base"

    if not DB_PATH.exists():
        print(f"Error: Database not found at {DB_PATH}")
        return

    print(f"Connecting to database at {DB_PATH}...")
    
    try:
        # Connect in read-only mode just to be safe
        conn = sqlite3.connect(f"file:{DB_PATH}?mode=ro", uri=True)
        # Handle non-UTF-8 characters by replacing them
        conn.text_factory = lambda b: b.decode(errors="replace")
        cursor = conn.cursor()
        
        # Get all messages
        print("Fetching messages...")
        cursor.execute("SELECT content FROM messages WHERE content IS NOT NULL AND content != ''")
        rows = cursor.fetchall()
        
        if not rows:
            print("No messages found in database.")
            return

        print(f"Found {len(rows)} messages. Calculating tokens...")
        
        # Initialize tokenizer
        try:
            encoding = tiktoken.get_encoding(ENCODING_NAME)
        except Exception as e:
            print(f"Error loading tokenizer: {e}")
            print("Please ensure tiktoken is installed: pip install tiktoken")
            return

        total_tokens = 0
        
        for row in rows:
            content = row[0]
            # Simple token estimation
            token_count = len(encoding.encode(content))
            total_tokens += token_count

        # Calculate cost
        estimated_cost = (total_tokens / 1_000_000) * MODEL_PRICE_PER_1M_TOKENS
        
        print("\n=== Estimation Results ===")
        print(f"Total Messages: {len(rows)}")
        print(f"Total Tokens:   {total_tokens:,}")
        print(f"Price per 1M:   ${MODEL_PRICE_PER_1M_TOKENS:.2f}")
        print(f"Estimated Cost: ${estimated_cost:.6f}")
        print("==========================")
        
    except sqlite3.Error as e:
        print(f"Database error: {e}")
    finally:
        if 'conn' in locals():
            conn.close()

if __name__ == "__main__":
    estimate_cost()
