"""
GPT Image tool implementation.

Provides image generation and editing using OpenAI's gpt-image-1.5 model.
"""

import os
import base64
import tempfile
import requests
from typing import Any, Dict, List, Optional
from .base import Tool


class GPTImageTool(Tool):
    """Image generation and editing tool using OpenAI's gpt-image-1.5 model."""
    
    # Valid options
    VALID_SIZES = ["1024x1024", "1536x1024", "1024x1536", "auto"]
    VALID_QUALITIES = ["low", "medium", "high", "auto"]
    VALID_FORMATS = ["png", "jpeg", "webp"]
    VALID_BACKGROUNDS = ["opaque", "transparent", "auto"]
    VALID_FIDELITIES = ["low", "high"]
    
    def __init__(self):
        """Initialize GPT Image tool."""
        self.api_key = os.environ.get("OPENAI_API_KEY")
        self.freeimage_api_key = os.environ.get("FREEIMAGE_API_KEY")
        self.upload_url = "https://freeimage.host/api/1/upload"
        self.base_url = "https://api.openai.com/v1/images"
    
    @property
    def name(self) -> str:
        return "gpt_image"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "gpt_image",
            "description": """Generate or edit images using OpenAI's gpt-image-1.5 model (state of the art).

CAPABILITIES:
- Generate images from text prompts with excellent text rendering
- Edit existing images using reference images (up to 5 with high fidelity)
- Inpainting: edit specific areas using a mask
- Transparent backgrounds for sprites/logos
- Multiple quality levels and sizes

USE THIS WHEN:
- User wants highest quality AI image generation
- User needs accurate text in images (signs, labels, etc.)
- User wants to combine/edit multiple reference images
- User needs transparent backgrounds
- User asks for "OpenAI image" or "GPT image"

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
                        "description": "Optional: URLs of input images for editing. First 5 images get high fidelity preservation with gpt-image-1.5"
                    },
                    "mask_url": {
                        "type": "string",
                        "description": "Optional: URL of mask image for inpainting. White areas will be edited, black areas preserved. Must have alpha channel."
                    },
                    "size": {
                        "type": "string",
                        "enum": ["1024x1024", "1536x1024", "1024x1536", "auto"],
                        "description": "Output size. 1024x1024 (square), 1536x1024 (landscape), 1024x1536 (portrait), or auto. Default: auto"
                    },
                    "quality": {
                        "type": "string",
                        "enum": ["low", "medium", "high", "auto"],
                        "description": "Image quality. Higher = better but slower/more expensive. Default: auto"
                    },
                    "output_format": {
                        "type": "string",
                        "enum": ["png", "jpeg", "webp"],
                        "description": "Output format. Use png/webp for transparency. jpeg is fastest. Default: png"
                    },
                    "background": {
                        "type": "string",
                        "enum": ["opaque", "transparent", "auto"],
                        "description": "Background type. transparent only works with png/webp and quality medium/high. Default: auto"
                    },
                    "output_compression": {
                        "type": "integer",
                        "minimum": 0,
                        "maximum": 100,
                        "description": "Compression level for jpeg/webp (0-100%). Lower = smaller file. Only for jpeg/webp."
                    },
                    "input_fidelity": {
                        "type": "string",
                        "enum": ["low", "high"],
                        "description": "Input image fidelity. high preserves more detail from input images (faces, logos). Default: low"
                    },
                    "n": {
                        "type": "integer",
                        "minimum": 1,
                        "maximum": 4,
                        "description": "Number of images to generate (1-4). Default: 1"
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
    
    def _upload_to_freeimage(self, image_bytes: bytes, format: str = "png") -> str:
        """Upload image bytes to freeimage.host and return URL."""
        if not self.freeimage_api_key:
            raise ValueError("FREEIMAGE_API_KEY not configured")
        
        suffix = f".{format}"
        with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
            tmp.write(image_bytes)
            tmp_path = tmp.name
        
        try:
            with open(tmp_path, "rb") as f:
                upload_response = requests.post(
                    self.upload_url,
                    data={
                        "key": self.freeimage_api_key,
                        "action": "upload",
                        "format": "json"
                    },
                    files={"source": f}
                )
            
            if upload_response.status_code != 200:
                raise ValueError(f"Upload failed: {upload_response.status_code} {upload_response.text}")
            
            upload_result = upload_response.json()
            
            if upload_result.get("status_code") != 200:
                error_msg = upload_result.get("error", {}).get("message", "Unknown error")
                raise ValueError(f"Upload failed: {error_msg}")
            
            return upload_result["image"]["url"]
        finally:
            os.unlink(tmp_path)
    
    def execute(
        self,
        prompt: str,
        input_image_urls: Optional[List[str]] = None,
        mask_url: Optional[str] = None,
        size: str = "auto",
        quality: str = "auto",
        output_format: str = "png",
        background: str = "auto",
        output_compression: Optional[int] = None,
        input_fidelity: str = "low",
        n: int = 1,
        **kwargs
    ) -> str:
        """
        Generate or edit images using gpt-image-1.5.
        
        Args:
            prompt: Text description of image to generate or edit to apply
            input_image_urls: Optional list of input image URLs for editing
            mask_url: Optional mask URL for inpainting
            size: Output size (1024x1024, 1536x1024, 1024x1536, auto)
            quality: Quality level (low, medium, high, auto)
            output_format: Output format (png, jpeg, webp)
            background: Background type (opaque, transparent, auto)
            output_compression: Compression for jpeg/webp (0-100)
            input_fidelity: Input fidelity (low, high)
            n: Number of images to generate (1-4)
            
        Returns:
            URL(s) to the generated/edited image(s) or error message
        """
        if not self.api_key:
            return "Error: OPENAI_API_KEY not configured"
        
        # Validate parameters
        if size not in self.VALID_SIZES:
            return f"Error: Invalid size. Must be one of: {', '.join(self.VALID_SIZES)}"
        if quality not in self.VALID_QUALITIES:
            return f"Error: Invalid quality. Must be one of: {', '.join(self.VALID_QUALITIES)}"
        if output_format not in self.VALID_FORMATS:
            return f"Error: Invalid format. Must be one of: {', '.join(self.VALID_FORMATS)}"
        if background not in self.VALID_BACKGROUNDS:
            return f"Error: Invalid background. Must be one of: {', '.join(self.VALID_BACKGROUNDS)}"
        if input_fidelity not in self.VALID_FIDELITIES:
            return f"Error: Invalid input_fidelity. Must be one of: {', '.join(self.VALID_FIDELITIES)}"
        if n < 1 or n > 4:
            return "Error: n must be between 1 and 4"
        
        # Validate transparency requirements
        if background == "transparent":
            if output_format not in ["png", "webp"]:
                return "Error: Transparent background requires png or webp format"
            if quality == "low":
                return "Error: Transparent background requires medium or high quality"
        
        # Validate compression - only matters for jpeg/webp, ignore for png
        if output_compression is not None and output_compression > 0:
            if output_format not in ["jpeg", "webp"]:
                # Silently ignore compression for png instead of erroring
                output_compression = None
            elif output_compression > 100:
                return "Error: output_compression must be between 0 and 100"
        
        try:
            headers = {
                "Authorization": f"Bearer {self.api_key}",
            }
            
            # Determine if this is generation or edit
            is_edit = bool(input_image_urls) or bool(mask_url)
            
            if is_edit:
                return self._edit_image(
                    prompt=prompt,
                    input_image_urls=input_image_urls or [],
                    mask_url=mask_url,
                    size=size,
                    quality=quality,
                    output_format=output_format,
                    background=background,
                    output_compression=output_compression,
                    input_fidelity=input_fidelity,
                    n=n,
                    headers=headers
                )
            else:
                return self._generate_image(
                    prompt=prompt,
                    size=size,
                    quality=quality,
                    output_format=output_format,
                    background=background,
                    output_compression=output_compression,
                    n=n,
                    headers=headers
                )
                
        except Exception as e:
            return f"Error: {str(e)}"

    def _generate_image(
        self,
        prompt: str,
        size: str,
        quality: str,
        output_format: str,
        background: str,
        output_compression: Optional[int],
        n: int,
        headers: Dict[str, str]
    ) -> str:
        """Generate a new image from prompt."""
        payload = {
            "model": "gpt-image-1.5",
            "prompt": prompt,
            "n": n,
            "moderation": "low",  # Always use low moderation
        }
        
        # Add optional parameters (only if not default/auto to let API decide)
        if size != "auto":
            payload["size"] = size
        if quality != "auto":
            payload["quality"] = quality
        if background != "auto":
            payload["background"] = background
        if output_format != "png":
            payload["output_format"] = output_format
        if output_compression is not None:
            payload["output_compression"] = output_compression
        
        response = requests.post(
            f"{self.base_url}/generations",
            headers={**headers, "Content-Type": "application/json"},
            json=payload,
            timeout=180  # Up to 2 minutes for complex prompts
        )
        
        if response.status_code != 200:
            return f"Error: {response.status_code} - {response.text}"
        
        result = response.json()
        
        # Process and upload images
        urls = []
        for item in result.get("data", []):
            image_b64 = item.get("b64_json")
            if image_b64:
                image_bytes = base64.b64decode(image_b64)
                url = self._upload_to_freeimage(image_bytes, output_format)
                urls.append(url)
        
        if not urls:
            return "Error: No images generated"
        
        if len(urls) == 1:
            return urls[0]
        return " | ".join(urls)
    
    def _edit_image(
        self,
        prompt: str,
        input_image_urls: List[str],
        mask_url: Optional[str],
        size: str,
        quality: str,
        output_format: str,
        background: str,
        output_compression: Optional[int],
        input_fidelity: str,
        n: int,
        headers: Dict[str, str]
    ) -> str:
        """Edit existing images."""
        # Download input images
        image_files = []
        temp_files = []
        
        try:
            for i, url in enumerate(input_image_urls):
                img_bytes = self._download_image(url)
                # Detect format from content or default to png
                suffix = ".png"
                if img_bytes[:2] == b'\xff\xd8':
                    suffix = ".jpg"
                elif img_bytes[:4] == b'RIFF':
                    suffix = ".webp"
                
                tmp = tempfile.NamedTemporaryFile(suffix=suffix, delete=False)
                tmp.write(img_bytes)
                tmp.close()
                temp_files.append(tmp.name)
                image_files.append(("image[]", (f"image{i}{suffix}", open(tmp.name, "rb"))))
            
            # Download mask if provided
            mask_file = None
            if mask_url:
                mask_bytes = self._download_image(mask_url)
                mask_tmp = tempfile.NamedTemporaryFile(suffix=".png", delete=False)
                mask_tmp.write(mask_bytes)
                mask_tmp.close()
                temp_files.append(mask_tmp.name)
                mask_file = ("mask", ("mask.png", open(mask_tmp.name, "rb")))
            
            # Build form data
            data = {
                "model": "gpt-image-1.5",
                "prompt": prompt,
                "n": str(n),
                "moderation": "low",  # Always use low moderation
            }
            
            if size != "auto":
                data["size"] = size
            if quality != "auto":
                data["quality"] = quality
            if background != "auto":
                data["background"] = background
            if output_format != "png":
                data["output_format"] = output_format
            if output_compression is not None:
                data["output_compression"] = str(output_compression)
            if input_fidelity != "low":
                data["input_fidelity"] = input_fidelity
            
            files = image_files
            if mask_file:
                files.append(mask_file)
            
            response = requests.post(
                f"{self.base_url}/edits",
                headers=headers,
                data=data,
                files=files,
                timeout=180
            )
            
            if response.status_code != 200:
                return f"Error: {response.status_code} - {response.text}"
            
            result = response.json()
            
            # Process and upload images
            urls = []
            for item in result.get("data", []):
                image_b64 = item.get("b64_json")
                if image_b64:
                    image_bytes = base64.b64decode(image_b64)
                    url = self._upload_to_freeimage(image_bytes, output_format)
                    urls.append(url)
            
            if not urls:
                return "Error: No images generated"
            
            if len(urls) == 1:
                return urls[0]
            return " | ".join(urls)
            
        finally:
            # Clean up temp files
            for f in image_files:
                try:
                    f[1][1].close()
                except:
                    pass
            if mask_file:
                try:
                    mask_file[1][1].close()
                except:
                    pass
            for tmp_path in temp_files:
                try:
                    os.unlink(tmp_path)
                except:
                    pass
