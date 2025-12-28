
import chromadb
import os
from pathlib import Path
from dotenv import load_dotenv
from openai import OpenAI

# Load environment variables
load_dotenv()

def run_test():
    CHROMA_PATH = Path("data/chroma_db")
    COLLECTION_NAME = "chat_history"
    MODEL_NAME = "text-embedding-3-small"
    RELEVANCE_THRESHOLD = 0.55  # Messages with distance < this are considered relevant
    
    api_key = os.getenv("OPENAI_API_KEY")
    if not api_key:
        print("Error: OPENAI_API_KEY not found.")
        return

    print(f"Connecting to ChromaDB at {CHROMA_PATH}...")
    client = chromadb.PersistentClient(path=str(CHROMA_PATH))
    collection = client.get_collection(name=COLLECTION_NAME)
    openai_client = OpenAI(api_key=api_key)

    # IRC-style queries that users would actually ask
    tests = [
        {
            "query": "what did mathisen say about cats?",
            "keywords": ["cat", "kitten", "feline"],
            "expected_user": "Mathisen",
            "description": "User-specific query (Mathisen's cat discussions)",
            "context": "User wants to recall what Mathisen said about cats"
        },
        {
            "query": "when did de-facto talk about upgrading bella?",
            "keywords": ["upgrade", "bella", "brain", "update"],
            "expected_user": "de-facto",
            "description": "Event recall with user filter (de-facto + bella upgrade)",
            "context": "User wants to know when de-facto mentioned upgrading bella"
        },
        {
            "query": "xml parsing issues discussed in the channel",
            "keywords": ["xml", "parse", "cdata", "xslt", "valid"],
            "expected_user": None,  # Any user
            "description": "Technical topic search (XML parsing)",
            "context": "User wants to find technical discussions about XML"
        }
    ]

    print(f"\nRunning {len(tests)} IRC-style semantic search tests...")
    print(f"Relevance threshold: {RELEVANCE_THRESHOLD} (lower distance = more relevant)\n")

    total_passed = 0
    
    for idx, test in enumerate(tests, 1):
        print(f"{'='*80}")
        print(f"Test {idx}: {test['description']}")
        print(f"{'='*80}")
        print(f"IRC Query: '{test['query']}'")
        print(f"Context: {test['context']}")
        print(f"Looking for keywords: {', '.join(test['keywords'])}")
        if test['expected_user']:
            print(f"Filtering for user: {test['expected_user']}")
        print()
        
        # Generate embedding
        response = openai_client.embeddings.create(
            input=test['query'],
            model=MODEL_NAME
        )
        query_embedding = response.data[0].embedding
        
        # Query ChromaDB with optional user filter
        query_params = {
            "query_embeddings": [query_embedding],
            "n_results": 20,
            "include": ['documents', 'metadatas', 'distances']
        }
        
        # Add metadata filter if user is specified
        if test['expected_user']:
            query_params["where"] = {"nick": test['expected_user']}
        
        results = collection.query(**query_params)
        
        # Evaluate results
        relevant_count = 0
        keyword_matches = 0
        
        print("Top Results:")
        print("-" * 80)
        
        for i, doc in enumerate(results['documents'][0]):
            meta = results['metadatas'][0][i]
            distance = results['distances'][0][i]
            
            timestamp = meta.get('timestamp', 'Unknown')[:19]
            nick = meta.get('nick', 'Unknown')
            
            # Check relevance
            is_relevant = distance < RELEVANCE_THRESHOLD
            if is_relevant:
                relevant_count += 1
            
            # Check keyword matches
            doc_lower = doc.lower()
            matched_keywords = [kw for kw in test['keywords'] if kw in doc_lower]
            if matched_keywords:
                keyword_matches += 1
            
            # Visual indicators
            relevance_mark = "✅ RELEVANT" if is_relevant else "⚠️  WEAK"
            keyword_mark = f"[Keywords: {', '.join(matched_keywords)}]" if matched_keywords else ""
            
            print(f"{i+1}. [{timestamp}] {nick}")
            print(f"   Dist: {distance:.4f} {relevance_mark} {keyword_mark}")
            print(f"   > {doc[:120]}{'...' if len(doc) > 120 else ''}")
            print()

        # Determine test success
        # Pass if we have at least 3 relevant results OR at least 2 with keyword matches
        passed = (relevant_count >= 3) or (keyword_matches >= 2)
        
        print("-" * 80)
        print(f"Results Summary:")
        print(f"  • Relevant matches (dist < {RELEVANCE_THRESHOLD}): {relevant_count}/8")
        print(f"  • Keyword matches: {keyword_matches}/8")
        print(f"  • Test Status: {'✅ PASS' if passed else '❌ FAIL'}")
        
        if passed:
            total_passed += 1
        
        print()

    # Final summary
    print(f"{'='*80}")
    print(f"FINAL RESULTS: {total_passed}/{len(tests)} tests passed")
    print(f"{'='*80}")
    
    if total_passed == len(tests):
        print("🎉 All tests passed! Semantic search is working well.")
    else:
        print(f"⚠️  {len(tests) - total_passed} test(s) need improvement.")
            
if __name__ == "__main__":
    run_test()
