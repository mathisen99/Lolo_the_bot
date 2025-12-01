"""
URL fetching tool implementation.

Fetches and extracts readable content from web pages, bypassing bot protection.
"""

import os
import re
import time
from typing import Any, Dict, Optional
from urllib.parse import urlparse
from .base import Tool


class FetchUrlTool(Tool):
    """URL fetching tool with bot protection bypass and content extraction."""
    
    # Max characters to return (roughly 1 token = 4 chars, aim for ~3000 tokens max)
    MAX_CONTENT_LENGTH = 12000
    
    # Request timeout in seconds
    REQUEST_TIMEOUT = 15
    
    # User agents to rotate through
    USER_AGENTS = [
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
    ]
    
    def __init__(self):
        """Initialize fetch URL tool."""
        self._session = None
    
    @property
    def name(self) -> str:
        return "fetch_url"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "fetch_url",
            "description": "Fetch content from a URL. Works with web pages (extracts readable text), PDFs (extracts text), code files (Python, JS, etc.), JSON, XML, YAML, Markdown, and other text-based content. Use when a user shares a link. Limited to ~8000 characters.",
            "parameters": {
                "type": "object",
                "properties": {
                    "url": {
                        "type": "string",
                        "description": "The URL to fetch content from"
                    }
                },
                "required": ["url"],
                "additionalProperties": False
            }
        }

    def _get_session(self):
        """Get or create a requests session with browser-like headers."""
        if self._session is None:
            import requests
            import random
            
            self._session = requests.Session()
            self._session.headers.update({
                "User-Agent": random.choice(self.USER_AGENTS),
                "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
                "Accept-Language": "en-US,en;q=0.5",
                "Accept-Encoding": "gzip, deflate, br",
                "DNT": "1",
                "Connection": "keep-alive",
                "Upgrade-Insecure-Requests": "1",
                "Sec-Fetch-Dest": "document",
                "Sec-Fetch-Mode": "navigate",
                "Sec-Fetch-Site": "none",
                "Sec-Fetch-User": "?1",
                "Cache-Control": "max-age=0",
            })
        return self._session
    
    def _validate_url(self, url: str) -> tuple[bool, str]:
        """Validate URL is safe to fetch."""
        try:
            parsed = urlparse(url)
            
            # Must have scheme
            if parsed.scheme not in ("http", "https"):
                return False, "URL must start with http:// or https://"
            
            # Must have host
            if not parsed.netloc:
                return False, "Invalid URL: no host found"
            
            # Block local/private IPs
            host = parsed.netloc.split(":")[0].lower()
            blocked = ["localhost", "127.0.0.1", "0.0.0.0", "::1", "10.", "192.168.", "172.16."]
            for b in blocked:
                if host.startswith(b) or host == b.rstrip("."):
                    return False, "Cannot fetch local/private URLs"
            
            return True, ""
        except Exception as e:
            return False, f"Invalid URL: {str(e)}"
    
    def _extract_text_simple(self, html: str) -> str:
        """Extract text from HTML using regex (no external deps)."""
        # Remove script and style elements
        html = re.sub(r'<script[^>]*>.*?</script>', '', html, flags=re.DOTALL | re.IGNORECASE)
        html = re.sub(r'<style[^>]*>.*?</style>', '', html, flags=re.DOTALL | re.IGNORECASE)
        html = re.sub(r'<noscript[^>]*>.*?</noscript>', '', html, flags=re.DOTALL | re.IGNORECASE)
        
        # Remove HTML comments
        html = re.sub(r'<!--.*?-->', '', html, flags=re.DOTALL)
        
        # Remove all HTML tags
        text = re.sub(r'<[^>]+>', ' ', html)
        
        # Decode common HTML entities
        entities = {
            '&nbsp;': ' ', '&amp;': '&', '&lt;': '<', '&gt;': '>',
            '&quot;': '"', '&#39;': "'", '&apos;': "'",
            '&mdash;': '—', '&ndash;': '–', '&hellip;': '...',
        }
        for entity, char in entities.items():
            text = text.replace(entity, char)
        
        # Clean up whitespace
        text = re.sub(r'\s+', ' ', text)
        text = text.strip()
        
        return text

    def _extract_text_bs4(self, html: str, base_url: str = "") -> str:
        """Extract text using BeautifulSoup, preserving links as markdown."""
        try:
            from bs4 import BeautifulSoup
            from urllib.parse import urljoin, urlparse
            
            soup = BeautifulSoup(html, 'html.parser')
            
            # Remove unwanted elements
            for element in soup(['script', 'style', 'noscript', 'iframe', 'svg']):
                element.decompose()
            
            # Convert links to markdown format before extracting text
            base_parsed = urlparse(base_url) if base_url else None
            
            for a_tag in soup.find_all('a', href=True):
                href = a_tag['href']
                link_text = a_tag.get_text(strip=True)
                
                # Skip empty links, anchors, javascript, mailto
                if not href or not link_text:
                    continue
                if href.startswith(('#', 'javascript:', 'mailto:', 'tel:')):
                    continue
                
                # Resolve relative URLs
                if base_url and not href.startswith(('http://', 'https://')):
                    full_url = urljoin(base_url, href)
                else:
                    full_url = href
                
                # Use relative path if same domain (saves chars)
                if base_parsed and full_url.startswith(('http://', 'https://')):
                    link_parsed = urlparse(full_url)
                    if link_parsed.netloc == base_parsed.netloc:
                        # Same domain - use relative path
                        display_url = link_parsed.path
                        if link_parsed.query:
                            display_url += '?' + link_parsed.query
                    else:
                        display_url = full_url
                else:
                    display_url = full_url
                
                # Replace link with markdown format
                a_tag.replace_with(f'[{link_text}]({display_url})')
            
            # Try to find main content
            main_content = None
            for selector in ['article', 'main', '[role="main"]', '.content', '.post', '.article']:
                main_content = soup.select_one(selector)
                if main_content:
                    break
            
            if main_content:
                text = main_content.get_text(separator=' ', strip=True)
            else:
                text = soup.get_text(separator=' ', strip=True)
            
            # Clean up whitespace (but preserve markdown links)
            text = re.sub(r'\s+', ' ', text)
            return text.strip()
            
        except ImportError:
            return self._extract_text_simple(html)
    
    def _fetch_with_requests(self, url: str) -> tuple[bool, str, str]:
        """Fetch URL using requests library.
        
        Returns:
            Tuple of (success, content_or_error, content_type)
        """
        import requests
        
        session = self._get_session()
        
        try:
            response = session.get(
                url,
                timeout=self.REQUEST_TIMEOUT,
                allow_redirects=True,
                verify=True
            )
            response.raise_for_status()
            
            # Check content type - allow any text-based content
            content_type = response.headers.get('content-type', '').lower()
            
            # Block binary/media content types (but allow PDF)
            blocked_types = ['image/', 'video/', 'audio/', 'application/octet-stream', 
                           'application/zip', 'application/gzip',
                           'application/x-tar', 'application/x-rar', 'font/']
            for blocked in blocked_types:
                if blocked in content_type:
                    return False, f"Cannot fetch binary content: {content_type}", ""
            
            # Handle PDF separately (return raw bytes marker)
            if 'application/pdf' in content_type or url.lower().endswith('.pdf'):
                return True, "__PDF_BINARY__", content_type
            
            return True, response.text, content_type
            
        except requests.exceptions.Timeout:
            return False, "Request timed out", ""
        except requests.exceptions.TooManyRedirects:
            return False, "Too many redirects", ""
        except requests.exceptions.SSLError:
            return False, "SSL certificate error", ""
        except requests.exceptions.RequestException as e:
            return False, f"Request failed: {str(e)}", ""
    
    def _fetch_with_cloudscraper(self, url: str) -> tuple[bool, str]:
        """Fetch URL using cloudscraper to bypass Cloudflare."""
        try:
            import cloudscraper
            
            scraper = cloudscraper.create_scraper(
                browser={'browser': 'chrome', 'platform': 'windows', 'mobile': False}
            )
            
            response = scraper.get(url, timeout=self.REQUEST_TIMEOUT)
            response.raise_for_status()
            
            return True, response.text
            
        except ImportError:
            return False, "cloudscraper not available"
        except Exception as e:
            return False, f"Cloudscraper failed: {str(e)}"

    def _fetch_pdf(self, url: str) -> tuple[bool, str]:
        """Fetch and extract text from a PDF URL.
        
        Returns:
            Tuple of (success, text_or_error)
        """
        import requests
        
        session = self._get_session()
        
        try:
            response = session.get(
                url,
                timeout=self.REQUEST_TIMEOUT,
                allow_redirects=True,
                verify=True
            )
            response.raise_for_status()
            
            # Extract text from PDF
            return self._extract_pdf_text(response.content)
            
        except requests.exceptions.Timeout:
            return False, "Request timed out"
        except requests.exceptions.RequestException as e:
            return False, f"Request failed: {str(e)}"

    def _extract_pdf_text(self, pdf_bytes: bytes) -> tuple[bool, str]:
        """Extract text content from PDF bytes.
        
        Returns:
            Tuple of (success, text_or_error)
        """
        try:
            from pypdf import PdfReader
            import io
            
            reader = PdfReader(io.BytesIO(pdf_bytes))
            
            text_parts = []
            for page_num, page in enumerate(reader.pages, 1):
                page_text = page.extract_text()
                if page_text:
                    text_parts.append(f"[Page {page_num}]\n{page_text}")
            
            if not text_parts:
                return False, "PDF contains no extractable text (may be scanned/image-based)"
            
            return True, "\n\n".join(text_parts)
            
        except ImportError:
            return False, "PDF support not available (pypdf not installed)"
        except Exception as e:
            return False, f"Failed to parse PDF: {str(e)}"

    def _is_html_content(self, content_type: str, content: str) -> bool:
        """Check if content is HTML based on content-type or content inspection."""
        if 'text/html' in content_type or 'application/xhtml' in content_type:
            return True
        # Also check content itself for HTML markers if content-type is ambiguous
        if not content_type or 'text/plain' in content_type:
            # Check for common HTML markers
            content_start = content[:500].lower().strip()
            if content_start.startswith('<!doctype') or content_start.startswith('<html'):
                return True
            if '<head>' in content_start or '<body>' in content_start:
                return True
        return False

    def execute(self, url: str, **kwargs) -> str:
        """
        Fetch and extract content from a URL.
        
        Args:
            url: The URL to fetch
            **kwargs: Additional parameters (ignored)
            
        Returns:
            Extracted text content or error message
        """
        # Validate URL
        valid, error = self._validate_url(url)
        if not valid:
            return f"Error: {error}"
        
        # Try fetching with different methods
        content = None
        content_type = ""
        
        # First try regular requests
        success, result, content_type = self._fetch_with_requests(url)
        if success:
            content = result
        else:
            # Check if it looks like Cloudflare block
            if "403" in result or "cloudflare" in result.lower():
                # Try cloudscraper
                success, result = self._fetch_with_cloudscraper(url)
                if success:
                    content = result
        
        if content is None:
            return f"Error: Failed to fetch URL - {result}"
        
        # Handle PDF content
        if content == "__PDF_BINARY__":
            success, pdf_text = self._fetch_pdf(url)
            if not success:
                return f"Error: {pdf_text}"
            
            # Truncate if too long
            if len(pdf_text) > self.MAX_CONTENT_LENGTH:
                pdf_text = pdf_text[:self.MAX_CONTENT_LENGTH] + "... [content truncated]"
            
            return f"Title: PDF Document\n\nContent:\n{pdf_text}"
        
        # Check for Cloudflare challenge page (only relevant for HTML)
        if self._is_html_content(content_type, content):
            if "challenge-platform" in content or "cf-browser-verification" in content:
                # Try cloudscraper as fallback
                success, result = self._fetch_with_cloudscraper(url)
                if success:
                    content = result
                else:
                    return "Error: Page is protected by Cloudflare and could not be bypassed"
        
        # Handle content based on type
        if self._is_html_content(content_type, content):
            # Extract text from HTML (pass URL for resolving relative links)
            try:
                text = self._extract_text_bs4(content, base_url=url)
            except Exception:
                text = self._extract_text_simple(content)
            
            if not text:
                return "Error: No readable content found on page"
            
            # Get page title if possible
            title_match = re.search(r'<title[^>]*>([^<]+)</title>', content, re.IGNORECASE)
            title = title_match.group(1).strip() if title_match else None
        else:
            # Non-HTML content (code, JSON, plain text, etc.) - return as-is
            text = content.strip()
            title = None
            
            # Try to determine content type for context
            if 'json' in content_type or 'application/json' in content_type:
                title = "JSON Content"
            elif 'python' in content_type or url.endswith('.py'):
                title = "Python Code"
            elif 'javascript' in content_type or url.endswith('.js'):
                title = "JavaScript Code"
            elif 'xml' in content_type or url.endswith('.xml'):
                title = "XML Content"
            elif 'yaml' in content_type or url.endswith(('.yml', '.yaml')):
                title = "YAML Content"
            elif 'markdown' in content_type or url.endswith('.md'):
                title = "Markdown Content"
        
        # Truncate if too long
        if len(text) > self.MAX_CONTENT_LENGTH:
            text = text[:self.MAX_CONTENT_LENGTH] + "... [content truncated]"
        
        if title:
            return f"Title: {title}\n\nContent:\n{text}"
        else:
            return f"Content:\n{text}"
