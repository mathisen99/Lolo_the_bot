
import chromadb
import os
from pathlib import Path
from dotenv import load_dotenv
from openai import OpenAI

# Load environment variables
load_dotenv()

def verify():
    CHROMA_PATH = Path("data/chroma_db")
    COLLECTION_NAME = "chat_history"
    MODEL_NAME = "text-embedding-3-small"
    
    api_key = os.getenv("OPENAI_API_KEY")
    if not api_key:
        print("Error: OPENAI_API_KEY not found.")
        return

    if not CHROMA_PATH.exists():
        print(f"Error: ChromaDB not found at {CHROMA_PATH}")
        return

    print(f"Connecting to ChromaDB at {CHROMA_PATH}...")
    client = chromadb.PersistentClient(path=str(CHROMA_PATH))
    
    try:
        collection = client.get_collection(name=COLLECTION_NAME)
        count = collection.count()
        print(f"Collection '{COLLECTION_NAME}' contains {count} documents.")
        
        if count == 0:
            print("Collection is empty!")
            return

        # Initialize OpenAI for embedding generation
        openai_client = OpenAI(api_key=api_key)

        # Test Query 1: Semantic Search
        query_text = "russian"
        print(f"\n--- Test Query: '{query_text}' ---")
        
        # Generate embedding for query
        response = openai_client.embeddings.create(
            input=query_text,
            model=MODEL_NAME
        )
        query_embedding = response.data[0].embedding
        
        results = collection.query(
            query_embeddings=[query_embedding],
            n_results=3
        )
        
        for i, doc in enumerate(results['documents'][0]):
            meta = results['metadatas'][0][i]
            print(f"\nResult {i+1}:")
            print(f"Content: {doc}")
            print(f"Metadata: {meta}")

        # Test Query 2: Metadata Filtering
        print(f"\n--- Test Query: Metadata Filter (is_bot=true) ---")
        # Just getting the last inserted bot message essentially, or random
        results = collection.get(
            where={"is_bot": True},
            limit=3
        )
        
        # collection.get returns dictionaries with 'ids', 'embeddings', 'metadatas', 'documents'
        # but they are flat lists, not nested list of lists like query()
        if results['ids']:
             for i, doc in enumerate(results['documents']):
                meta = results['metadatas'][i]
                print(f"\nResult {i+1}:")
                print(f"Content: {doc}")
                print(f"Metadata: {meta}")
        else:
            print("No bot messages found.")

    except Exception as e:
        print(f"Error verifying ChromaDB: {e}")

if __name__ == "__main__":
    verify()
