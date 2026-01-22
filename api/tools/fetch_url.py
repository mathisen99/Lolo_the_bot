"""
URL fetching tool implementation.

Fetches and extracts readable content from web pages, bypassing bot protection.
"""

import re
from typing import Any, Dict, List, Optional, Tuple
from urllib.parse import urlparse
from .base import Tool


class FetchUrlTool(Tool):
    """URL fetching tool with bot protection bypass and content extraction."""
    
    # Max characters to return (roughly 1 token = 4 chars, aim for ~3000 tokens max)
    MAX_CONTENT_LENGTH = 25000
    
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
            "description": """Fetch content from a URL. Works with web pages (extracts readable text), PDFs, code files (Python, JS, etc.), JSON, XML, YAML, Markdown, and other text-based content.

TRUNCATION HANDLING (~25000 char limit):
When content is truncated (indicated by [TRUNCATED] at the end), you can retrieve more content by:
1. Note the last topic/heading/function/paragraph shown before truncation
2. Call fetch_url again with the SAME url but use search_term with a keyword from that last section or the next expected topic
3. Repeat if needed to gather all relevant information

Example workflow for large documents:
- First fetch returns: "...Chapter 3: Authentication\n[TRUNCATED]"
- Follow-up: fetch_url(url, search_term="Chapter 4") or fetch_url(url, search_term="Authorization")
- This retrieves the next relevant section without re-fetching already seen content

For code files: Use function names, class names, or unique identifiers from the truncation point.
For articles: Use section headings, key terms, or phrases visible near the end.
For PDFs: Use page numbers like "Page 5" or section titles.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "url": {
                        "type": "string",
                        "description": "The URL to fetch content from"
                    },
                    "search_term": {
                        "type": ["string", "null"],
                        "description": "Search for specific content and return only matching sections. Use to: (1) Find specific functions/topics/sections in large files, (2) Continue reading truncated content by searching for the next section/heading/function after the cutoff point, (3) Jump to relevant parts without re-reading already seen content. Extracts full code blocks or paragraphs containing matches. Case-insensitive."
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
    
    def _validate_url(self, url: str) -> Tuple[bool, str]:
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
    
    def _fetch_with_requests(self, url: str) -> Tuple[bool, str, str]:
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
    
    def _fetch_with_cloudscraper(self, url: str) -> Tuple[bool, str]:
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

    def _fetch_pdf(self, url: str) -> Tuple[bool, str]:
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

    def _extract_pdf_text(self, pdf_bytes: bytes) -> Tuple[bool, str]:
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

    def _is_code_file(self, url: str, content_type: str) -> bool:
        """Check if content is a code file based on URL or content-type."""
        code_extensions = (
            '.py', '.js', '.ts', '.jsx', '.tsx', '.c', '.cpp', '.h', '.hpp',
            '.java', '.go', '.rs', '.rb', '.php', '.cs', '.swift', '.kt',
            '.scala', '.sh', '.bash', '.zsh', '.pl', '.lua', '.r', '.m'
        )
        return url.lower().endswith(code_extensions)

    def _extract_code_blocks(self, content: str, search_term: str) -> List[Tuple[int, str]]:
        """Extract code blocks (functions, classes, etc.) containing the search term.
        
        Returns list of (line_number, block_content) tuples.
        """
        lines = content.split('\n')
        results = []
        search_lower = search_term.lower()
        
        # Find all lines containing the search term
        match_lines = []
        for i, line in enumerate(lines):
            if search_lower in line.lower():
                match_lines.append(i)
        
        if not match_lines:
            return []
        
        # For each match, try to extract the containing block
        processed_ranges = set()
        
        for match_line in match_lines:
            # Skip if this line is already in a processed range
            if any(start <= match_line <= end for start, end in processed_ranges):
                continue
            
            # Find block boundaries by looking for function/class definitions
            # and tracking indentation
            block_start = match_line
            block_end = match_line
            
            # Look backwards for block start (function def, class def, or significant decrease in indent)
            base_indent = len(lines[match_line]) - len(lines[match_line].lstrip())
            
            for i in range(match_line - 1, max(0, match_line - 200), -1):
                line = lines[i]
                if not line.strip():  # Skip empty lines
                    continue
                
                line_indent = len(line) - len(line.lstrip())
                stripped = line.strip()
                
                # Check for block starters
                is_block_start = (
                    stripped.startswith(('def ', 'class ', 'function ', 'func ', 'fn ')) or
                    stripped.startswith(('public ', 'private ', 'protected ', 'static ')) or
                    re.match(r'^(async\s+)?(def|function|class)\s+', stripped) or
                    re.match(r'^[a-zA-Z_]\w*\s*\([^)]*\)\s*[{:]?\s*$', stripped) or  # function signature
                    stripped.startswith(('#define', 'template', 'namespace'))
                )
                
                if is_block_start and line_indent <= base_indent:
                    block_start = i
                    base_indent = line_indent
                    break
                elif line_indent < base_indent and stripped:
                    # Significant dedent - might be end of outer block
                    block_start = i + 1
                    break
            
            # Look forwards for block end
            for i in range(match_line + 1, min(len(lines), match_line + 200)):
                line = lines[i]
                if not line.strip():
                    continue
                
                line_indent = len(line) - len(line.lstrip())
                stripped = line.strip()
                
                # Check for next block start at same or lower indent
                is_block_start = (
                    stripped.startswith(('def ', 'class ', 'function ', 'func ', 'fn ')) or
                    stripped.startswith(('public ', 'private ', 'protected ', 'static ')) or
                    re.match(r'^(async\s+)?(def|function|class)\s+', stripped)
                )
                
                if is_block_start and line_indent <= base_indent:
                    block_end = i - 1
                    break
                elif line_indent < base_indent and stripped and not stripped.startswith(('}', ')', ']')):
                    block_end = i - 1
                    break
                else:
                    block_end = i
            
            # Ensure we have at least some context
            block_start = max(0, block_start - 2)
            block_end = min(len(lines) - 1, block_end + 2)
            
            # Skip if too small
            if block_end - block_start < 2:
                # Fall back to context around match
                block_start = max(0, match_line - 30)
                block_end = min(len(lines) - 1, match_line + 50)
            
            processed_ranges.add((block_start, block_end))
            block_content = '\n'.join(lines[block_start:block_end + 1])
            results.append((block_start + 1, block_content))  # 1-indexed line numbers
        
        return results

    def _extract_text_sections(self, content: str, search_term: str) -> List[Tuple[int, str]]:
        """Extract paragraphs/sections containing the search term from plain text.
        
        Returns list of (position, section_content) tuples.
        """
        search_lower = search_term.lower()
        content_lower = content.lower()
        results = []
        
        # Split into paragraphs (double newline) or sentences for HTML-extracted text
        if '\n\n' in content:
            sections = content.split('\n\n')
            separator = '\n\n'
        else:
            # For single-line text (like extracted HTML), split by sentences
            sections = re.split(r'(?<=[.!?])\s+', content)
            separator = ' '
        
        # Find sections containing the search term
        position = 0
        for i, section in enumerate(sections):
            if search_lower in section.lower():
                # Include surrounding sections for context
                start_idx = max(0, i - 1)
                end_idx = min(len(sections), i + 2)
                context = separator.join(sections[start_idx:end_idx])
                
                # Limit individual section size
                if len(context) > 3000:
                    # Find the match and extract around it
                    match_pos = section.lower().find(search_lower)
                    start = max(0, match_pos - 1000)
                    end = min(len(section), match_pos + len(search_term) + 1500)
                    context = "..." + section[start:end] + "..."
                
                results.append((position, context))
            
            position += len(section) + len(separator)
        
        return results

    def _search_content(self, content: str, search_term: str, is_code: bool) -> str:
        """Search content and return only relevant sections."""
        if is_code:
            matches = self._extract_code_blocks(content, search_term)
            if not matches:
                return f"No matches found for '{search_term}' in the code."
            
            result_parts = [f"Found {len(matches)} section(s) matching '{search_term}':\n"]
            total_chars = len(result_parts[0])
            
            for line_num, block in matches:
                header = f"\n--- Lines starting at {line_num} ---\n"
                section = header + block + "\n"
                
                if total_chars + len(section) > self.MAX_CONTENT_LENGTH:
                    result_parts.append(f"\n[Additional matches truncated - {len(matches) - len(result_parts) + 1} more sections not shown]")
                    break
                
                result_parts.append(section)
                total_chars += len(section)
            
            return ''.join(result_parts)
        else:
            matches = self._extract_text_sections(content, search_term)
            if not matches:
                return f"No matches found for '{search_term}' in the content."
            
            result_parts = [f"Found {len(matches)} section(s) matching '{search_term}':\n"]
            total_chars = len(result_parts[0])
            
            for i, (pos, section) in enumerate(matches, 1):
                header = f"\n--- Match {i} ---\n"
                section_text = header + section + "\n"
                
                if total_chars + len(section_text) > self.MAX_CONTENT_LENGTH:
                    result_parts.append(f"\n[Additional matches truncated - {len(matches) - i + 1} more sections not shown]")
                    break
                
                result_parts.append(section_text)
                total_chars += len(section_text)
            
            return ''.join(result_parts)

    def execute(self, url: str, search_term: Optional[str] = None, **kwargs) -> str:
        """
        Fetch and extract content from a URL.
        
        Args:
            url: The URL to fetch
            search_term: Optional search term to extract only matching sections
            **kwargs: Additional parameters (ignored)
            
        Returns:
            Extracted text content or error message
        """
        # Validate URL
        valid, error = self._validate_url(url)
        if not valid:
            return f"Error: {error}"
        
        # Clean search term
        if search_term:
            search_term = search_term.strip()
            if not search_term:
                search_term = None
        
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
            
            # Apply search if specified
            if search_term:
                searched_content = self._search_content(pdf_text, search_term, is_code=False)
                return f"Title: PDF Document (searched for '{search_term}')\n\n{searched_content}"
            
            # Truncate if too long
            truncated = False
            original_len = len(pdf_text)
            if original_len > self.MAX_CONTENT_LENGTH:
                pdf_text = pdf_text[:self.MAX_CONTENT_LENGTH]
                truncated = True
            
            result = f"Title: PDF Document\n\nContent:\n{pdf_text}"
            if truncated:
                result += f"\n\n[TRUNCATED: Content exceeded {self.MAX_CONTENT_LENGTH} character limit. Original was ~{original_len} chars. Only the first portion is shown. TIP: Use search_term parameter to find specific content.]"
            return result
        
        # Check for Cloudflare challenge page (only relevant for HTML)
        if self._is_html_content(content_type, content):
            if "challenge-platform" in content or "cf-browser-verification" in content:
                # Try cloudscraper as fallback
                success, result = self._fetch_with_cloudscraper(url)
                if success:
                    content = result
                else:
                    return "Error: Page is protected by Cloudflare and could not be bypassed"
        
        # Determine if this is a code file
        is_code = self._is_code_file(url, content_type)
        
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
            elif url.endswith(('.c', '.cpp', '.h', '.hpp')):
                title = "C/C++ Code"
            elif 'xml' in content_type or url.endswith('.xml'):
                title = "XML Content"
            elif 'yaml' in content_type or url.endswith(('.yml', '.yaml')):
                title = "YAML Content"
            elif 'markdown' in content_type or url.endswith('.md'):
                title = "Markdown Content"
            elif is_code:
                title = "Code File"
        
        # Apply search if specified
        if search_term:
            searched_content = self._search_content(text, search_term, is_code=is_code)
            title_suffix = f" (searched for '{search_term}')"
            if title:
                return f"Title: {title}{title_suffix}\n\n{searched_content}"
            else:
                return f"Content{title_suffix}:\n\n{searched_content}"
        
        # Truncate if too long
        truncated = False
        original_len = len(text)
        if original_len > self.MAX_CONTENT_LENGTH:
            text = text[:self.MAX_CONTENT_LENGTH]
            truncated = True
        
        if title:
            result = f"Title: {title}\n\nContent:\n{text}"
        else:
            result = f"Content:\n{text}"
        
        if truncated:
            result += f"\n\n[TRUNCATED: Content exceeded {self.MAX_CONTENT_LENGTH} character limit. Original was ~{original_len} chars. Only the first portion is shown. TIP: Use search_term parameter to find specific content.]"
        
        return result
