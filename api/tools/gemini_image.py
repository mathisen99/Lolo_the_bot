"""
Gemini Image tool implementation.

Provides image generation and editing using Google's Gemini 3 Pro Image Preview model.
"""

import io
import os
import base64
import requests
from typing import Any, Dict, List, Optional, Tuple
from PIL import Image
from .base import Tool
from api.utils.botbin import upload_to_botbin


class GeminiImageTool(Tool):
    """Image generation and editing tool using Google's Gemini 3 Pro Image Preview model."""
    
    # Valid options - stored as (ratio_string, width/height_value) for matching
    VALID_ASPECT_RATIOS = ["1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9"]
    ASPECT_RATIO_VALUES = {
        "1:1": 1.0,
        "2:3": 2/3,
        "3:2": 3/2,
        "3:4": 3/4,
        "4:3": 4/3,
        "4:5": 4/5,
        "5:4": 5/4,
        "9:16": 9/16,
        "16:9": 16/9,
        "21:9": 21/9,
    }
    VALID_RESOLUTIONS = ["1K", "2K", "4K"]
    
    def __init__(self):
        """Initialize Gemini Image tool."""
        self.api_key = os.environ.get("GEMINI_API_KEY")
        self.base_url = "https://generativelanguage.googleapis.com/v1beta/models"
        self.model = "gemini-3-pro-image-preview"
    
    @property
    def name(self) -> str:
        return "gemini_image"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "gemini_image",
            "description": """Generate or edit images using Google's Gemini 3 Pro Image Preview model.

CAPABILITIES:
- Generate high-quality images from text prompts
- Edit existing images using reference images (up to 14 total: 6 objects + 5 humans)
- Multi-turn conversational image refinement
- High-fidelity text rendering (logos, diagrams, posters)
- Advanced reasoning for complex compositions
- Grounding with Google Search for real-time data

ASPECT RATIO PRESERVATION:
When editing images, the original aspect ratio is automatically detected and matched to the closest supported ratio. ONLY specify aspect_ratio if user explicitly requests a different one.

USE THIS WHEN:
- User wants Google/Gemini image generation
- User needs complex multi-image compositions
- User wants iterative image refinement
- User asks for "gemini image" specifically
- User needs infographics or diagrams with accurate text

Returns a URL to the generated/edited image.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "prompt": {
                        "type": "string",
                        "description": "Text description of the image to generate or the edit to apply"
                    },
                    "input_image_urls": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Optional: URLs of input images for editing. Up to 14 images (6 objects + 5 humans for character consistency)"
                    },
                    "aspect_ratio": {
                        "type": "string",
                        "enum": ["1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9", "auto"],
                        "description": "Output aspect ratio. Use 'auto' (default) to preserve input image aspect ratio when editing. Only specify explicitly if user requests a different ratio."
                    },
                    "resolution": {
                        "type": "string",
                        "enum": ["1K", "2K", "4K"],
                        "description": "Output resolution. Higher = better quality but slower. Default: 1K"
                    }
                },
                "required": ["prompt"],
                "additionalProperties": False
            }
        }

    def _download_image(self, url: str) -> bytes:
        """Download image from URL and return bytes."""
        response = requests.get(url, timeout=30)
        response.raise_for_status()
        return response.content
    
    def _get_image_dimensions(self, img_bytes: bytes) -> Optional[Tuple[int, int]]:
        """Extract image dimensions from bytes. Returns (width, height) or None."""
        try:
            img = Image.open(io.BytesIO(img_bytes))
            return img.size
        except Exception:
            return None
    
    def _find_closest_aspect_ratio(self, width: int, height: int) -> str:
        """Find the closest supported aspect ratio for given dimensions."""
        if height == 0:
            return "1:1"
        
        target_ratio = width / height
        
        best_match = "1:1"
        best_diff = float('inf')
        
        for ratio_str, ratio_val in self.ASPECT_RATIO_VALUES.items():
            diff = abs(target_ratio - ratio_val)
            if diff < best_diff:
                best_diff = diff
                best_match = ratio_str
        
        return best_match
    
    def _get_mime_type(self, url: str, img_bytes: bytes) -> str:
        """Determine MIME type from URL or image bytes."""
        url_lower = url.lower()
        if url_lower.endswith('.png'):
            return "image/png"
        elif url_lower.endswith('.gif'):
            return "image/gif"
        elif url_lower.endswith('.webp'):
            return "image/webp"
        elif url_lower.endswith(('.jpg', '.jpeg')):
            return "image/jpeg"
        
        # Check magic bytes
        if img_bytes[:8] == b'\x89PNG\r\n\x1a\n':
            return "image/png"
        elif img_bytes[:2] == b'\xff\xd8':
            return "image/jpeg"
        elif img_bytes[:6] in (b'GIF87a', b'GIF89a'):
            return "image/gif"
        elif img_bytes[:4] == b'RIFF' and img_bytes[8:12] == b'WEBP':
            return "image/webp"
        
        # Default to JPEG
        return "image/jpeg"
    
    def _upload_image(self, image_bytes: bytes, format: str = "png") -> str:
        """Upload image bytes to botbin.net and return URL."""
        return upload_to_botbin(image_bytes, f"image.{format}")
    
    def execute(
        self,
        prompt: str,
        input_image_urls: Optional[List[str]] = None,
        aspect_ratio: str = "auto",
        resolution: str = "1K",
        **kwargs
    ) -> str:
        """
        Generate or edit images using Gemini 3 Pro Image Preview.
        
        Args:
            prompt: Text description of image to generate or edit to apply
            input_image_urls: Optional list of input image URLs for editing
            aspect_ratio: Output aspect ratio (or "auto" to detect from input)
            resolution: Output resolution (1K, 2K, 4K)
            
        Returns:
            URL to the generated/edited image or error message
        """
        if not self.api_key:
            return "Error: GEMINI_API_KEY not configured"
        
        # Validate resolution
        if resolution not in self.VALID_RESOLUTIONS:
            return f"Error: Invalid resolution. Must be one of: {', '.join(self.VALID_RESOLUTIONS)}"
        
        # Handle aspect ratio - detect from input if "auto" and editing
        detected_ratio = None
        if aspect_ratio == "auto":
            if input_image_urls:
                # Detect from first input image
                try:
                    first_img_bytes = self._download_image(input_image_urls[0])
                    dims = self._get_image_dimensions(first_img_bytes)
                    if dims:
                        detected_ratio = self._find_closest_aspect_ratio(dims[0], dims[1])
                        print(f"[gemini_image] Detected aspect ratio {dims[0]}x{dims[1]} -> {detected_ratio}")
                except Exception as e:
                    print(f"[gemini_image] Failed to detect aspect ratio: {e}")
            
            # Default to 1:1 for generation or if detection failed
            aspect_ratio = detected_ratio or "1:1"
        
        # Validate aspect ratio
        if aspect_ratio not in self.VALID_ASPECT_RATIOS:
            return f"Error: Invalid aspect_ratio. Must be one of: {', '.join(self.VALID_ASPECT_RATIOS)}"
        
        try:
            # Build content parts
            parts = [{"text": prompt}]
            
            # Add input images if provided
            if input_image_urls:
                if len(input_image_urls) > 14:
                    return "Error: Maximum 14 input images allowed"
                
                for url in input_image_urls:
                    try:
                        img_bytes = self._download_image(url)
                        mime_type = self._get_mime_type(url, img_bytes)
                        img_b64 = base64.b64encode(img_bytes).decode('utf-8')
                        parts.append({
                            "inline_data": {
                                "mime_type": mime_type,
                                "data": img_b64
                            }
                        })
                    except Exception as e:
                        return f"Error downloading image {url}: {str(e)}"
            
            # Build request payload
            payload = {
                "contents": [{
                    "parts": parts
                }],
                "generationConfig": {
                    "responseModalities": ["TEXT", "IMAGE"],
                    "imageConfig": {
                        "aspectRatio": aspect_ratio,
                        "imageSize": resolution
                    }
                }
            }
            
            # Make API request
            response = requests.post(
                f"{self.base_url}/{self.model}:generateContent",
                headers={
                    "x-goog-api-key": self.api_key,
                    "Content-Type": "application/json"
                },
                json=payload,
                timeout=180  # Up to 3 minutes for complex generations
            )
            
            if response.status_code != 200:
                error_text = response.text
                return f"Error: {response.status_code} - {error_text}"
            
            result = response.json()
            
            # Extract image from response
            candidates = result.get("candidates", [])
            if not candidates:
                return "Error: No response generated"
            
            content = candidates[0].get("content", {})
            response_parts = content.get("parts", [])
            
            text_response = ""
            image_url = None
            
            for part in response_parts:
                if "text" in part:
                    text_response = part["text"]
                elif "inlineData" in part:
                    inline_data = part["inlineData"]
                    img_b64 = inline_data.get("data")
                    if img_b64:
                        img_bytes = base64.b64decode(img_b64)
                        # Detect format from mimeType or default to png
                        mime_type = inline_data.get("mimeType", "image/png")
                        ext_map = {
                            "image/png": "png",
                            "image/jpeg": "jpeg",
                            "image/jpg": "jpeg",
                            "image/webp": "webp",
                            "image/gif": "gif",
                        }
                        ext = ext_map.get(mime_type, "png")
                        image_url = self._upload_image(img_bytes, ext)
            
            if image_url:
                if text_response:
                    return f"{image_url} | {text_response}"
                return image_url
            elif text_response:
                return f"No image generated. Model response: {text_response}"
            else:
                return "Error: No image or text in response"
                
        except requests.Timeout:
            return "Error: Request timed out. Try a simpler prompt or lower resolution."
        except Exception as e:
            return f"Error: {str(e)}"
