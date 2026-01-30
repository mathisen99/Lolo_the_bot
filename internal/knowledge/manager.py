"""
Knowledge Base Manager.

Handles ingestion of documents into ChromaDB for RAG retrieval.
"""

import io
import os
import time
import hashlib
import re
from pathlib import Path
from typing import List, Dict, Any, Optional, Tuple
from urllib.parse import urlparse

import chromadb
from openai import OpenAI

# For PDF extraction
try:
    from pypdf import PdfReader
    PYPDF_AVAILABLE = True
except ImportError:
    PYPDF_AVAILABLE = False

# For HTML extraction
try:
    from bs4 import BeautifulSoup
    BS4_AVAILABLE = True
except ImportError:
    BS4_AVAILABLE = False


class KnowledgeBaseManager:
    """Manages the knowledge base ChromaDB collection."""
    
    # Chunk settings
    CHUNK_SIZE = 1000  # Characters per chunk
    CHUNK_OVERLAP = 150  # Overlap between chunks
    
    # Embedding model
    EMBEDDING_MODEL = "text-embedding-3-small"
    
    # Collection name
    COLLECTION_NAME = "knowledge_base"
    
    def __init__(self, chroma_path: Optional[str] = None, openai_api_key: Optional[str] = None):
        """
        Initialize the Knowledge Base Manager.
        
        Args:
            chroma_path: Path to ChromaDB storage. Defaults to data/chroma_db
            openai_api_key: OpenAI API key for embeddings.
        """
        self._chroma_path = chroma_path or str(Path("data/chroma_db"))
        self._openai_api_key = openai_api_key or os.getenv("OPENAI_API_KEY")
        
        if not self._openai_api_key:
            raise ValueError("OPENAI_API_KEY not set")
        
        self._openai_client = OpenAI(api_key=self._openai_api_key)
        self._chroma_client = chromadb.PersistentClient(path=self._chroma_path)
        self._collection = self._chroma_client.get_or_create_collection(
            name=self.COLLECTION_NAME,
            metadata={"hnsw:space": "cosine"}
        )
    
    # --- Ingestion ---
    
    def learn_from_url(self, url: str) -> Dict[str, Any]:
        """
        Ingest content from a URL into the knowledge base.
        
        Args:
            url: The URL to fetch and ingest.
            
        Returns:
            Dict with 'success', 'message', 'title', 'chunks_added'.
        """
        # Check if already ingested
        existing = self._collection.get(where={"source_url": url}, limit=1)
        if existing and existing['ids']:
            return {
                "success": False,
                "message": f"URL already ingested. Use forget to re-ingest.",
                "title": None,
                "chunks_added": 0
            }
        
        # Fetch content
        fetch_result = self._fetch_content(url)
        if not fetch_result["success"]:
            return {
                "success": False,
                "message": fetch_result["error"],
                "title": None,
                "chunks_added": 0
            }
        
        text = fetch_result["text"]
        title = fetch_result.get("title") or self._extract_title_from_url(url)
        
        # Chunk the text
        chunks = self._chunk_text(text)
        
        if not chunks:
            return {
                "success": False,
                "message": "No content to ingest after processing.",
                "title": title,
                "chunks_added": 0
            }
        
        # Generate embeddings
        embeddings = self._generate_embeddings([c["text"] for c in chunks])
        
        if not embeddings:
            return {
                "success": False,
                "message": "Failed to generate embeddings.",
                "title": title,
                "chunks_added": 0
            }
        
        # Prepare data for ChromaDB
        url_hash = hashlib.md5(url.encode()).hexdigest()[:8]
        timestamp = time.strftime("%Y-%m-%d %H:%M:%S")
        
        ids = [f"kb_{url_hash}_{i}" for i in range(len(chunks))]
        documents = [c["text"] for c in chunks]
        metadatas = [
            {
                "source_url": url,
                "title": title,
                "chunk_index": i,
                "total_chunks": len(chunks),
                "ingested_at": timestamp
            }
            for i in range(len(chunks))
        ]
        
        # Upsert into ChromaDB
        self._collection.add(
            ids=ids,
            embeddings=embeddings,
            documents=documents,
            metadatas=metadatas
        )
        
        return {
            "success": True,
            "message": f"Successfully learned '{title}'. Stored {len(chunks)} chunks.",
            "title": title,
            "chunks_added": len(chunks)
        }
    
    def forget_url(self, url: str) -> Dict[str, Any]:
        """
        Remove all chunks from a URL from the knowledge base.
        
        Args:
            url: The source URL to forget.
            
        Returns:
            Dict with 'success', 'message', 'chunks_removed'.
        """
        existing = self._collection.get(where={"source_url": url}, include=[])
        if not existing or not existing['ids']:
            return {
                "success": False,
                "message": "URL not found in knowledge base.",
                "chunks_removed": 0
            }
        
        ids_to_delete = existing['ids']
        self._collection.delete(ids=ids_to_delete)
        
        return {
            "success": True,
            "message": f"Removed {len(ids_to_delete)} chunks from URL.",
            "chunks_removed": len(ids_to_delete)
        }
    
    # --- Retrieval ---
    
    def search(self, query: str, n_results: int = 5) -> List[Dict[str, Any]]:
        """
        Search the knowledge base for relevant chunks.
        
        Args:
            query: The search query.
            n_results: Number of results to return.
            
        Returns:
            List of dicts with 'text', 'source_url', 'title', 'distance'.
        """
        # Generate query embedding
        embeddings = self._generate_embeddings([query])
        if not embeddings:
            return []
        
        results = self._collection.query(
            query_embeddings=embeddings,
            n_results=n_results,
            include=["documents", "metadatas", "distances"]
        )
        
        if not results or not results['ids'] or not results['ids'][0]:
            return []
        
        output = []
        for i, doc_id in enumerate(results['ids'][0]):
            output.append({
                "text": results['documents'][0][i],
                "source_url": results['metadatas'][0][i].get("source_url", "Unknown"),
                "title": results['metadatas'][0][i].get("title", "Unknown"),
                "distance": results['distances'][0][i] if results['distances'] else None
            })
        
        return output
    
    def list_sources(self) -> List[Dict[str, Any]]:
        """
        List all unique sources in the knowledge base.
        
        Returns:
            List of dicts with 'url', 'title', 'chunks'.
        """
        all_data = self._collection.get(include=["metadatas"])
        
        if not all_data or not all_data['ids']:
            return []
        
        sources = {}
        for meta in all_data['metadatas']:
            url = meta.get("source_url", "Unknown")
            if url not in sources:
                sources[url] = {
                    "url": url,
                    "title": meta.get("title", "Unknown"),
                    "chunks": 0
                }
            sources[url]["chunks"] += 1
        
        return list(sources.values())
    
    # --- Private Helpers ---
    
    def _fetch_content(self, url: str) -> Dict[str, Any]:
        """Fetch content from URL, supporting HTML and PDF."""
        import requests
        
        try:
            response = requests.get(url, timeout=30, headers={
                "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
            })
            response.raise_for_status()
        except requests.RequestException as e:
            return {"success": False, "error": f"Failed to fetch URL: {e}"}
        
        content_type = response.headers.get('content-type', '').lower()
        
        # Handle PDF
        if 'application/pdf' in content_type or url.lower().endswith('.pdf'):
            return self._extract_pdf_text(response.content)
        
        # Handle HTML
        if 'text/html' in content_type:
            return self._extract_html_text(response.text, url)
        
        # Handle plain text/markdown/etc
        return {
            "success": True,
            "text": response.text,
            "title": None
        }
    
    def _extract_pdf_text(self, pdf_bytes: bytes) -> Dict[str, Any]:
        """Extract text from PDF bytes."""
        if not PYPDF_AVAILABLE:
            return {"success": False, "error": "pypdf not installed"}
        
        try:
            reader = PdfReader(io.BytesIO(pdf_bytes))
            text_parts = []
            for page in reader.pages:
                page_text = page.extract_text()
                if page_text:
                    text_parts.append(page_text)
            
            if not text_parts:
                return {"success": False, "error": "PDF contains no extractable text"}
            
            # Try to get title from metadata
            title = None
            if reader.metadata and reader.metadata.title:
                title = reader.metadata.title
            
            return {
                "success": True,
                "text": "\n\n".join(text_parts),
                "title": title
            }
        except Exception as e:
            return {"success": False, "error": f"Failed to parse PDF: {e}"}
    
    def _extract_html_text(self, html: str, url: str) -> Dict[str, Any]:
        """Extract text from HTML."""
        title = None
        
        # Try to extract title
        title_match = re.search(r'<title[^>]*>([^<]+)</title>', html, re.IGNORECASE)
        if title_match:
            title = title_match.group(1).strip()
        
        if BS4_AVAILABLE:
            try:
                soup = BeautifulSoup(html, 'html.parser')
                
                # Remove unwanted elements
                for element in soup(['script', 'style', 'noscript', 'iframe', 'nav', 'footer', 'header']):
                    element.decompose()
                
                # Try to find main content
                main_content = None
                for selector in ['article', 'main', '[role="main"]', '.content', '.post']:
                    main_content = soup.select_one(selector)
                    if main_content:
                        break
                
                if main_content:
                    text = main_content.get_text(separator=' ', strip=True)
                else:
                    text = soup.get_text(separator=' ', strip=True)
                
                # Clean up whitespace
                text = re.sub(r'\s+', ' ', text).strip()
                
                return {"success": True, "text": text, "title": title}
            except Exception:
                pass
        
        # Fallback: regex-based extraction
        html = re.sub(r'<script[^>]*>.*?</script>', '', html, flags=re.DOTALL | re.IGNORECASE)
        html = re.sub(r'<style[^>]*>.*?</style>', '', html, flags=re.DOTALL | re.IGNORECASE)
        text = re.sub(r'<[^>]+>', ' ', html)
        text = re.sub(r'\s+', ' ', text).strip()
        
        return {"success": True, "text": text, "title": title}
    
    def _chunk_text(self, text: str) -> List[Dict[str, Any]]:
        """
        Split text into overlapping chunks.
        
        Returns list of dicts with 'text' and 'index'.
        """
        if not text:
            return []
        
        chunks = []
        start = 0
        text_len = len(text)
        
        while start < text_len:
            end = start + self.CHUNK_SIZE
            
            # Try to break at sentence boundary
            if end < text_len:
                # Look for sentence end within last 100 chars of chunk
                search_start = max(start, end - 100)
                last_period = text.rfind('. ', search_start, end)
                if last_period > start:
                    end = last_period + 1
            
            chunk_text = text[start:end].strip()
            if chunk_text:
                chunks.append({
                    "text": chunk_text,
                    "index": len(chunks)
                })
            
            # Move start with overlap
            start = end - self.CHUNK_OVERLAP
            if start >= text_len:
                break
        
        return chunks
    
    def _generate_embeddings(self, texts: List[str]) -> Optional[List[List[float]]]:
        """Generate embeddings for a list of texts."""
        try:
            response = self._openai_client.embeddings.create(
                input=texts,
                model=self.EMBEDDING_MODEL
            )
            return [data.embedding for data in response.data]
        except Exception as e:
            print(f"Embedding error: {e}")
            return None
    
    def _extract_title_from_url(self, url: str) -> str:
        """Extract a reasonable title from URL."""
        parsed = urlparse(url)
        path = parsed.path.rstrip('/')
        if path:
            # Get last path segment
            title = path.split('/')[-1]
            # Remove file extension
            title = re.sub(r'\.[^.]+$', '', title)
            # Replace separators with spaces
            title = re.sub(r'[-_]', ' ', title)
            return title.title() if title else parsed.netloc
        return parsed.netloc
