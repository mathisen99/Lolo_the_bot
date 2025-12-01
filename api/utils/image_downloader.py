"""
Robust image downloader utility.

Handles downloading images from URLs with common bypasses for bot blocks,
Cloudflare protection, and other anti-scraping measures.
"""

import requests
import tempfile
from pathlib import Path
from typing import Optional, Tuple
import time


class ImageDownloader:
    """Robust image downloader with anti-bot bypasses."""
    
    def __init__(self):
        """Initialize downloader with common headers."""
        self.session = requests.Session()
        
        # Common browser headers to bypass bot detection
        self.headers = {
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
            'Accept': 'image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8',
            'Accept-Language': 'en-US,en;q=0.9',
            'Accept-Encoding': 'gzip, deflate, br',
            'DNT': '1',
            'Connection': 'keep-alive',
            'Upgrade-Insecure-Requests': '1',
            'Sec-Fetch-Dest': 'image',
            'Sec-Fetch-Mode': 'no-cors',
            'Sec-Fetch-Site': 'cross-site',
            'Cache-Control': 'max-age=0',
        }
    
    def download_image(
        self, 
        url: str, 
        timeout: int = 30,
        max_retries: int = 3
    ) -> Tuple[Optional[Path], Optional[str]]:
        """
        Download an image from a URL to a temporary file.
        
        Args:
            url: Image URL to download
            timeout: Request timeout in seconds
            max_retries: Maximum number of retry attempts
            
        Returns:
            Tuple of (temp_file_path, error_message)
            If successful: (Path object, None)
            If failed: (None, error message)
        """
        last_error = None
        
        for attempt in range(max_retries):
            try:
                # Add delay between retries
                if attempt > 0:
                    time.sleep(2 ** attempt)  # Exponential backoff
                
                # Make request with headers
                response = self.session.get(
                    url,
                    headers=self.headers,
                    timeout=timeout,
                    allow_redirects=True,
                    stream=True
                )
                
                # Check for successful response
                if response.status_code == 403:
                    # Try with different User-Agent for Cloudflare
                    alt_headers = self.headers.copy()
                    alt_headers['User-Agent'] = 'Mozilla/5.0 (X11; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0'
                    
                    response = self.session.get(
                        url,
                        headers=alt_headers,
                        timeout=timeout,
                        allow_redirects=True,
                        stream=True
                    )
                
                response.raise_for_status()
                
                # Verify content type is an image
                content_type = response.headers.get('Content-Type', '').lower()
                if not content_type.startswith('image/'):
                    # Some servers don't set proper content-type, check by extension
                    if not any(url.lower().endswith(ext) for ext in ['.png', '.jpg', '.jpeg', '.webp', '.gif']):
                        return None, f"URL does not appear to be an image (Content-Type: {content_type})"
                
                # Determine file extension from content-type or URL
                extension = self._get_extension(content_type, url)
                
                # Create temporary file
                temp_file = tempfile.NamedTemporaryFile(
                    suffix=extension,
                    delete=False
                )
                
                # Download in chunks to handle large files
                total_size = 0
                max_size = 50 * 1024 * 1024  # 50MB limit
                
                for chunk in response.iter_content(chunk_size=8192):
                    if chunk:
                        total_size += len(chunk)
                        if total_size > max_size:
                            temp_file.close()
                            Path(temp_file.name).unlink(missing_ok=True)
                            return None, f"Image too large (>{max_size // (1024*1024)}MB)"
                        temp_file.write(chunk)
                
                temp_file.close()
                
                # Verify the file is a valid image
                try:
                    from PIL import Image
                    with Image.open(temp_file.name) as img:
                        img.verify()
                except Exception as e:
                    Path(temp_file.name).unlink(missing_ok=True)
                    return None, f"Downloaded file is not a valid image: {str(e)}"
                
                return Path(temp_file.name), None
                
            except requests.exceptions.Timeout:
                last_error = f"Request timed out after {timeout} seconds"
            except requests.exceptions.ConnectionError:
                last_error = "Connection failed - server may be unreachable"
            except requests.exceptions.HTTPError as e:
                if e.response.status_code == 403:
                    last_error = "Access forbidden - image may be protected by anti-bot measures"
                elif e.response.status_code == 404:
                    last_error = "Image not found (404)"
                else:
                    last_error = f"HTTP error {e.response.status_code}"
            except Exception as e:
                last_error = f"Download failed: {str(e)}"
        
        # All retries failed
        return None, f"Failed after {max_retries} attempts. Last error: {last_error}"
    
    def _get_extension(self, content_type: str, url: str) -> str:
        """
        Determine file extension from content-type or URL.
        
        Args:
            content_type: HTTP Content-Type header
            url: Image URL
            
        Returns:
            File extension with leading dot (e.g., '.png')
        """
        # Map content-type to extension
        type_map = {
            'image/png': '.png',
            'image/jpeg': '.jpg',
            'image/jpg': '.jpg',
            'image/webp': '.webp',
            'image/gif': '.gif',
        }
        
        # Try content-type first
        for mime_type, ext in type_map.items():
            if mime_type in content_type.lower():
                return ext
        
        # Fall back to URL extension
        url_lower = url.lower()
        for ext in ['.png', '.jpg', '.jpeg', '.webp', '.gif']:
            if ext in url_lower:
                return ext
        
        # Default to .jpg
        return '.jpg'
    
    def cleanup(self, file_path: Optional[Path]) -> None:
        """
        Clean up temporary file.
        
        Args:
            file_path: Path to temporary file to delete
        """
        if file_path and file_path.exists():
            try:
                file_path.unlink()
            except Exception:
                pass  # Ignore cleanup errors
