
import sqlite3
import chromadb
import os
import time
import argparse
from pathlib import Path
from openai import OpenAI
from dotenv import load_dotenv

# Load environment variables
load_dotenv()

def migrate(limit=None, batch_size=100):
    # Configuration
    DB_PATH = Path("data/bot.db")
    CHROMA_PATH = Path("data/chroma_db")
    COLLECTION_NAME = "chat_history"
    MODEL_NAME = "text-embedding-3-small"
    
    api_key = os.getenv("OPENAI_API_KEY")
    if not api_key:
        print("Error: OPENAI_API_KEY not found in environment variables.")
        return

    if not DB_PATH.exists():
        print(f"Error: Database not found at {DB_PATH}")
        return

    # Initialize OpenAI client
    client = OpenAI(api_key=api_key)

    # Initialize ChromaDB
    print(f"Initializing ChromaDB at {CHROMA_PATH}...")
    chroma_client = chromadb.PersistentClient(path=str(CHROMA_PATH))
    collection = chroma_client.get_or_create_collection(
        name=COLLECTION_NAME,
        metadata={"hnsw:space": "cosine"}
    )
    
    # Check for existing data to enable incremental updates
    last_id = 0
    if collection.count() > 0:
        print("Checking existing data for incremental update...")
        # Fetch all IDs to find the latest one (efficient enough for <1M records)
        existing = collection.get(include=[])
        if existing and existing['ids']:
            # Extract IDs from "msg_123" format
            try:
                # Filter for valid format just in case
                valid_ids = [int(x.split('_')[1]) for x in existing['ids'] if x.startswith('msg_') and x.split('_')[1].isdigit()]
                if valid_ids:
                    last_id = max(valid_ids)
                    print(f"Found {len(existing['ids'])} existing documents. Last ID: {last_id}")
                    print(f"resuming migration from message ID {last_id}...")
            except Exception as e:
                print(f"Warning: Could not determine last ID from existing data ({e}). Starting fresh check...")
    
    try:
        conn = sqlite3.connect(f"file:{DB_PATH}?mode=ro", uri=True)
        # Handle non-UTF-8 characters by replacing them
        conn.text_factory = lambda b: b.decode(errors="replace")
        cursor = conn.cursor()
        
        # Determine query based on limit and incremental state
        query = f"SELECT id, timestamp, channel, nick, content, is_bot FROM messages WHERE content IS NOT NULL AND content != '' AND id > {last_id}"
        if limit:
            query += f" LIMIT {limit}"
            
        print("Fetching new messages from SQLite...")
        cursor.execute(query)
        rows = cursor.fetchall()
        
        if not rows:
            print("No new messages found to migrate.")
            return

        total_messages = len(rows)
        print(f"Found {total_messages} messages. Starting migration...")
        
        # Batch processing
        for i in range(0, total_messages, batch_size):
            batch = rows[i:i + batch_size]
            
            # Prepare data for ChromaDB
            ids = []
            documents = []
            metadatas = []
            
            # Prepare batch for embedding
            texts_to_embed = []
            
            for row in batch:
                msg_id, timestamp, channel, nick, content, is_bot = row
                
                # Create unique ID for Chroma
                chroma_id = f"msg_{msg_id}"
                
                ids.append(chroma_id)
                documents.append(content)
                texts_to_embed.append(content)
                metadatas.append({
                    "original_id": msg_id,
                    "timestamp": timestamp,
                    "timestamp_unix": int(time.mktime(time.strptime(timestamp[:19], "%Y-%m-%d %H:%M:%S"))) if timestamp else 0,
                    "channel": channel if channel else "PM",
                    "nick": nick,
                    "is_bot": bool(is_bot)
                })

            try:
                # Generate embeddings
                # Note: ChromaDB client can handle embedding generation automatically if configured,
                # but we'll do it manually to ensure we use the specific model and key we want.
                response = client.embeddings.create(
                    input=texts_to_embed,
                    model=MODEL_NAME
                )
                
                embeddings = [data.embedding for data in response.data]
                
                # Add to ChromaDB
                collection.add(
                    ids=ids,
                    embeddings=embeddings,
                    metadatas=metadatas,
                    documents=documents
                )
                
                print(f"Processed {min(i + batch_size, total_messages)}/{total_messages} messages...")
                
            except Exception as e:
                print(f"Error processing batch starting at index {i}: {e}")
                # Optional: break or continue based on preference. 
                # For now we'll continue to try the next batch.
                continue
                
            # Sleep briefly to avoid rate limits
            time.sleep(0.5)
            
        print("\nMigration complete!")
        print(f"Total documents in collection: {collection.count()}")

    except sqlite3.Error as e:
        print(f"Database error: {e}")
    finally:
        if 'conn' in locals():
            conn.close()

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Migrate chat history to ChromaDB")
    parser.add_argument("--limit", type=int, help="Limit number of messages to migrate (for testing)")
    parser.add_argument("--batch-size", type=int, default=100, help="Batch size for processing")
    
    args = parser.parse_args()
    migrate(limit=args.limit, batch_size=args.batch_size)
