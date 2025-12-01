"""Image analysis tool definition and implementation."""
import base64
import os
from pathlib import Path
from typing import Dict, Optional
from PIL import Image
import io
import math
import json
from .base import Tool
from api.utils.image_downloader import ImageDownloader


def validate_image_format(file_path: Path) -> tuple[bool, Optional[str]]:
    """
    Validate that the image is in a supported format.
    
    Args:
        file_path: Path to the image file
    
    Returns:
        Tuple of (is_valid, error_message)
    """
    supported_formats = {'.png', '.jpg', '.jpeg', '.webp', '.gif'}
    
    # Check file extension
    extension = file_path.suffix.lower()
    if extension not in supported_formats:
        return False, f"Unsupported format: {extension}. Supported formats: PNG, JPEG, WEBP, GIF"
    
    try:
        # Open and validate with PIL
        with Image.open(file_path) as img:
            # Check if GIF is animated
            if extension == '.gif':
                try:
                    img.seek(1)  # Try to seek to second frame
                    return False, "Animated GIFs are not supported. Only non-animated GIFs are allowed."
                except EOFError:
                    # Only one frame, so it's not animated
                    pass
            
            # Verify it's a valid image
            img.verify()
        
        return True, None
        
    except Exception as e:
        return False, f"Invalid or corrupted image file: {str(e)}"


def validate_file_size(file_path: Path, max_size_mb: int = 50) -> tuple[bool, Optional[str]]:
    """
    Validate that the file size is within limits.
    
    Args:
        file_path: Path to the image file
        max_size_mb: Maximum file size in megabytes
    
    Returns:
        Tuple of (is_valid, error_message)
    """
    file_size = file_path.stat().st_size
    max_size_bytes = max_size_mb * 1024 * 1024
    
    if file_size > max_size_bytes:
        size_mb = file_size / (1024 * 1024)
        return False, f"File too large: {size_mb:.2f}MB (max: {max_size_mb}MB)"
    
    return True, None


def encode_image_to_base64(file_path: Path) -> str:
    """
    Encode an image file to base64 string.
    
    Args:
        file_path: Path to the image file
    
    Returns:
        Base64-encoded string of the image
    """
    with open(file_path, "rb") as image_file:
        return base64.b64encode(image_file.read()).decode("utf-8")


def calculate_image_tokens(file_path: Path, detail: str = "auto") -> int:
    """
    Calculate the token cost for an image based on its dimensions.
    Uses the formula from docs/image_usage.md for gpt-5.1 (same as gpt-5-mini).
    
    Args:
        file_path: Path to the image file
        detail: Detail level ('low', 'high', 'auto')
    
    Returns:
        Estimated token count for the image
    """
    # Low detail is always 85 tokens
    if detail == "low":
        return 85
    
    try:
        with Image.open(file_path) as img:
            width, height = img.size
        
        # Calculate patches needed (32px x 32px patches)
        raw_patches = math.ceil(width / 32) * math.ceil(height / 32)
        
        # If patches exceed 1536, scale down
        if raw_patches > 1536:
            # Calculate shrink factor
            r = math.sqrt(32 * 32 * 1536 / (width * height))
            
            # Adjust to fit whole patches
            resized_width = width * r
            resized_height = height * r
            
            width_patches = math.floor(resized_width / 32)
            height_patches = math.floor(resized_height / 32)
            
            # Scale again to fit width
            if width_patches > 0:
                scale_factor = width_patches / (resized_width / 32)
                resized_width = resized_width * scale_factor
                resized_height = resized_height * scale_factor
            
            # Calculate final patches
            patches = math.ceil(resized_width / 32) * math.ceil(resized_height / 32)
        else:
            patches = raw_patches
        
        # Cap at 1536 patches
        patches = min(patches, 1536)
        
        # Apply multiplier for gpt-5.1 (1.62)
        tokens = int(patches * 1.62)
        
        return tokens
        
    except Exception:
        # If we can't calculate, return a conservative estimate
        return 1000


def smart_detail_selection(file_path: Path, detail: str = "auto") -> str:
    """
    Intelligently select detail level based on image characteristics.
    Optimizes token usage while maintaining quality.
    
    Args:
        file_path: Path to the image file
        detail: Requested detail level
    
    Returns:
        Optimized detail level ('low' or 'high')
    """
    if detail != "auto":
        return detail
    
    try:
        with Image.open(file_path) as img:
            width, height = img.size
            
            # Use low detail for small images (< 512x512)
            if width < 512 and height < 512:
                return "low"
            
            # Use low detail for very large images to save tokens
            # (they'll be downscaled anyway)
            if width > 2048 or height > 2048:
                return "low"
            
            # Use high detail for medium-sized images where detail matters
            return "high"
    except:
        # Default to low if we can't determine
        return "low"


class ImageAnalysisTool(Tool):
    """Image analysis tool for analyzing images via OpenAI API."""
    
    def __init__(self):
        """Initialize image analysis tool with downloader."""
        self.downloader = ImageDownloader()
    
    @property
    def name(self) -> str:
        return "analyze_image"
    
    def get_definition(self) -> Dict:
        """
        Get tool definition for OpenAI API.
        
        Returns:
            Tool definition dict for custom function
        """
        return {
            "type": "function",
            "name": "analyze_image",
            "description": "Analyze images from URLs or file paths. Use when user shares an image link and asks about it (e.g., 'what's in this image', 'solve this puzzle', 'describe this'). Supports PNG, JPEG, WEBP, GIF (non-animated). Max 50MB. Returns image data for analysis.",
            "parameters": {
                "type": "object",
                "properties": {
                    "image_source": {
                        "type": "string",
                        "description": "File path or URL. Examples: '~/image.png', 'https://example.com/img.jpg'",
                    },
                    "detail": {
                        "type": ["string", "null"],
                        "enum": ["low", "high", "auto", None],
                        "description": "Detail level: 'low' (85 tokens, fast), 'high' (detailed), 'auto' (default)",
                    },
                    "question": {
                        "type": ["string", "null"],
                        "description": "Optional question about the image",
                    },
                },
                "required": ["image_source", "detail", "question"],
                "additionalProperties": False
            },
            "strict": True
        }
    
    def execute(self, image_source: str, detail: str = "auto", question: Optional[str] = None) -> str:
        """
        Analyze an image from a file path or URL.
        
        This function prepares the image for analysis by the OpenAI API.
        For URLs, downloads the image first to bypass bot blocks.
        Returns a JSON string that will be parsed by the service layer.
        
        Args:
            image_source: File path or URL to the image
            detail: Detail level ('low', 'high', 'auto')
            question: Optional specific question about the image
        
        Returns:
            JSON string with image data formatted for API and metadata
        """
        temp_file = None
        
        try:
            # Handle URLs by downloading first
            if image_source.startswith(("http://", "https://")):
                temp_file, error = self.downloader.download_image(image_source)
                
                if error:
                    return json.dumps({
                        "status": "error",
                        "error": f"Failed to download image: {error}",
                        "suggestion": "Check that the URL is accessible and points to a valid image."
                    })
                
                # Use the downloaded file
                file_path = temp_file
                source_type = "url"
                original_source = image_source
            else:
                # Handle local file path
                file_path = Path(image_source).expanduser()
                if not file_path.is_absolute():
                    cwd = Path(os.environ.get('ORIGINAL_CWD', os.getcwd()))
                    file_path = cwd / file_path
                
                source_type = "file"
                original_source = str(file_path)
            
            # Check if file exists
            if not file_path.exists():
                return json.dumps({
                    "status": "error",
                    "error": f"Image file not found: {file_path}",
                    "suggestion": "Check that the file path is correct and the file exists."
                })
            
            # Validate format
            is_valid, error = validate_image_format(file_path)
            if not is_valid:
                return json.dumps({
                    "status": "error",
                    "error": error,
                    "suggestion": "Ensure the image is in a supported format (PNG, JPEG, WEBP, non-animated GIF)."
                })
            
            # Validate size
            is_valid, error = validate_file_size(file_path)
            if not is_valid:
                return json.dumps({
                    "status": "error",
                    "error": error,
                    "suggestion": "Image must be under 50MB."
                })
            
            # Smart detail selection for auto mode
            if detail == "auto":
                detail = smart_detail_selection(file_path, detail)
            
            # Encode to base64
            base64_image = encode_image_to_base64(file_path)
            
            # Determine MIME type
            extension = file_path.suffix.lower()
            mime_types = {
                '.png': 'image/png',
                '.jpg': 'image/jpeg',
                '.jpeg': 'image/jpeg',
                '.webp': 'image/webp',
                '.gif': 'image/gif'
            }
            mime_type = mime_types.get(extension, 'image/jpeg')
            
            # Format as data URL
            data_url = f"data:{mime_type};base64,{base64_image}"
            
            # Calculate token cost
            token_cost = calculate_image_tokens(file_path, detail)
            
            # Build response
            result = {
                "status": "success",
                "image_data": {
                    "type": "input_image",
                    "image_url": data_url,
                    "detail": detail if detail != "auto" else None
                },
                "detail": detail,
                "question": question,
                "source_type": source_type,
                "source": original_source,
                "token_cost": token_cost
            }
            
            return json.dumps(result)
            
        except Exception as e:
            return json.dumps({
                "status": "error",
                "error": f"Failed to process image: {str(e)}",
                "suggestion": "Verify the image file is valid and accessible."
            })
        
        finally:
            # Clean up temporary file if it was downloaded
            if temp_file:
                self.downloader.cleanup(temp_file)
