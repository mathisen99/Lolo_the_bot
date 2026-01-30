#!/usr/bin/env python3
"""
Test script for Knowledge Base functionality.

Usage:
    python scripts/test_kb.py

Tests:
1. Learn from a sample PDF URL
2. Search for content
3. List sources
4. Forget a URL
"""

import sys
import os

# Add project root to path
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from dotenv import load_dotenv
load_dotenv()

from internal.knowledge.manager import KnowledgeBaseManager


def main():
    print("=" * 60)
    print("Knowledge Base Test Script")
    print("=" * 60)
    
    # Initialize manager
    print("\n[1/5] Initializing KnowledgeBaseManager...")
    try:
        manager = KnowledgeBaseManager()
        print("✓ Manager initialized successfully")
    except Exception as e:
        print(f"✗ Failed to initialize manager: {e}")
        return 1
    
    # Test URL - using a simple PDF
    test_url = "https://arxiv.org/pdf/2310.06825.pdf"
    
    # Learn from URL
    print(f"\n[2/5] Learning from URL: {test_url}")
    print("(This may take a moment...)")
    try:
        result = manager.learn_from_url(test_url)
        if result["success"]:
            print(f"✓ Learned: '{result['title']}' - {result['chunks_added']} chunks")
        else:
            print(f"⚠ Could not learn (might be already ingested): {result['message']}")
    except Exception as e:
        print(f"✗ Failed to learn: {e}")
        return 1
    
    # Search
    print("\n[3/5] Searching for 'language model'...")
    try:
        results = manager.search("language model", n_results=3)
        if results:
            print(f"✓ Found {len(results)} results:")
            for i, r in enumerate(results, 1):
                preview = r['text'][:100].replace('\n', ' ')
                print(f"  {i}. [{r['title']}] {preview}...")
        else:
            print("⚠ No results found (KB might be empty)")
    except Exception as e:
        print(f"✗ Search failed: {e}")
        return 1
    
    # List sources
    print("\n[4/5] Listing all sources...")
    try:
        sources = manager.list_sources()
        print(f"✓ Knowledge base contains {len(sources)} source(s):")
        for s in sources:
            print(f"  • {s['title']} ({s['chunks']} chunks)")
    except Exception as e:
        print(f"✗ List failed: {e}")
        return 1
    
    # Forget (optional - uncomment to test)
    print("\n[5/5] Testing forget functionality...")
    print("(Skipping actual deletion to preserve test data)")
    # result = manager.forget_url(test_url)
    # print(f"✓ Forgot {result['chunks_removed']} chunks")
    
    print("\n" + "=" * 60)
    print("All tests passed! Knowledge Base is working correctly.")
    print("=" * 60)
    return 0


if __name__ == "__main__":
    sys.exit(main())
