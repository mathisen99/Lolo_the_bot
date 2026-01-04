"""
GPT Image tool implementation.

Provides image generation and editing using OpenAI's gpt-image-1.5 model.
"""

import os
import base64
import tempfile
import io
import requests
from typing import Any, Dict, List, Optional, Tuple
from PIL import Image
from .base import Tool
from api.utils.botbin import upload_to_botbin


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
                        "description": "Output size. 1024x1024 (square), 1536x1024 (landscape), ei (portrait), or auto. Default: auto"
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
    
    def _get_image_dimensions(self, img_bytes: bytes) -> Optional[Tuple[int, int]]:
        """
        Extract image dimensions from bytes using PIL.
        Returns (width, height) or None if unable to determine.
        """
        try:
            img = Image.open(io.BytesIO(img_bytes))
            return img.size  # (width, height)
        except Exception:
            return None
    
    def _find_best_target_size(self, width: int, height: int) -> Tuple[str, Tuple[int, int]]:
        """
        Find the best API size that can contain the image without any scaling.
        Returns (api_size_string, (target_width, target_height))
        """
        # Valid API sizes
        sizes = [
            ("1024x1024", 1024, 1024),
            ("1536x1024", 1536, 1024),
            ("1024x1536", 1024, 1536),
        ]
        
        # Find the smallest size that can contain the image WITHOUT scaling
        best = None
        for size_str, tw, th in sizes:
            if width <= tw and height <= th:
                if best is None or (tw * th < best[1] * best[2]):
                    best = (size_str, tw, th)
        
        # If image is too large for ANY size, we MUST scale - pick best aspect ratio
        if best is None:
            aspect = width / height if height > 0 else 1.0
            if aspect > 1.25:
                best = ("1536x1024", 1536, 1024)
            elif aspect < 0.8:
                best = ("1024x1536", 1024, 1536)
            else:
                best = ("1024x1024", 1024, 1024)
        
        return (best[0], (best[1], best[2]))
    
    def _pad_image_to_size(self, img_bytes: bytes, target_size: Tuple[int, int]) -> Tuple[bytes, Tuple[int, int, int, int], Tuple[int, int], float]:
        """
        Pad image to target size, centering the original. Only scales if image exceeds target.
        Returns (padded_image_bytes, crop_box, original_size, scale_factor).
        crop_box is (left, top, right, bottom) - the region containing the original image.
        """
        img = Image.open(io.BytesIO(img_bytes))
        orig_width, orig_height = img.size
        target_width, target_height = target_size
        
        # Only scale if image is LARGER than target (unavoidable)
        scale = 1.0
        if orig_width > target_width or orig_height > target_height:
            scale = min(target_width / orig_width, target_height / orig_height)
            new_width = int(orig_width * scale)
            new_height = int(orig_height * scale)
            img = img.resize((new_width, new_height), Image.Resampling.LANCZOS)
            print(f"[gpt_image] WARNING: Image too large, scaled from {orig_width}x{orig_height} to {new_width}x{new_height}")
            scaled_width, scaled_height = new_width, new_height
        else:
            scaled_width, scaled_height = orig_width, orig_height
        
        # Calculate padding to center the image
        left = (target_width - scaled_width) // 2
        top = (target_height - scaled_height) // 2
        right = left + scaled_width
        bottom = top + scaled_height
        
        # Create padded image with white background
        if img.mode == 'RGBA':
            padded = Image.new('RGBA', (target_width, target_height), (255, 255, 255, 255))
        else:
            if img.mode != 'RGB':
                img = img.convert('RGB')
            padded = Image.new('RGB', (target_width, target_height), (255, 255, 255))
        
        padded.paste(img, (left, top))
        
        output = io.BytesIO()
        padded.save(output, format='PNG')
        
        return (output.getvalue(), (left, top, right, bottom), (orig_width, orig_height), scale)
    
    def _crop_to_original(self, img_bytes: bytes, crop_box: Tuple[int, int, int, int], original_size: Tuple[int, int], scale: float) -> bytes:
        """
        Remove padding from result, restoring original dimensions exactly.
        """
        img = Image.open(io.BytesIO(img_bytes))
        
        # Crop out the padding - this gives us the edited image at scaled size
        cropped = img.crop(crop_box)
        
        # If we had to scale down, scale back up to original size
        if scale < 1.0:
            cropped = cropped.resize(original_size, Image.Resampling.LANCZOS)
        
        output = io.BytesIO()
        cropped.save(output, format='PNG')
        return output.getvalue()
    
    def _upload_image(self, image_bytes: bytes, format: str = "png") -> str:
        """Upload image bytes to botbin.net and return URL."""
        return upload_to_botbin(image_bytes, f"image.{format}")
    
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
        
        # Validate compression - only valid for jpeg/webp, not png
        if output_compression is not None:
            if output_format == "png":
                # PNG doesn't support compression < 100, silently ignore
                output_compression = None
            elif output_compression < 0 or output_compression > 100:
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
                url = self._upload_image(image_bytes, output_format)
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
        """Edit existing images with automatic size preservation."""
        image_files = []
        temp_files = []
        
        # Track original dimensions and crop info for restoring size
        original_size: Optional[Tuple[int, int]] = None
        crop_box: Optional[Tuple[int, int, int, int]] = None
        scale_factor: float = 1.0
        api_size: Optional[str] = None
        preserve_size = (size == "auto")  # Only preserve when user wants auto
        
        try:
            for i, url in enumerate(input_image_urls):
                img_bytes = self._download_image(url)
                
                # For first image: detect size and pad if preserving dimensions
                if i == 0 and preserve_size:
                    dims = self._get_image_dimensions(img_bytes)
                    if dims:
                        original_size = dims
                        api_size, target_size = self._find_best_target_size(dims[0], dims[1])
                        print(f"[gpt_image] Original: {dims[0]}x{dims[1]} -> Padding to {api_size} for API")
                        
                        # Pad image to target size (no cropping, only padding)
                        img_bytes, crop_box, original_size, scale_factor = self._pad_image_to_size(img_bytes, target_size)
                
                # Save to temp file
                tmp = tempfile.NamedTemporaryFile(suffix=".png", delete=False)
                tmp.write(img_bytes)
                tmp.close()
                temp_files.append(tmp.name)
                image_files.append(("image[]", (f"image{i}.png", open(tmp.name, "rb"))))
            
            # Download and pad mask if provided
            mask_file = None
            if mask_url:
                mask_bytes = self._download_image(mask_url)
                
                # If preserving size and we padded the image, pad mask the same way
                if preserve_size and original_size:
                    _, target_size = self._find_best_target_size(original_size[0], original_size[1])
                    mask_bytes, _, _, _ = self._pad_image_to_size(mask_bytes, target_size)
                
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
                "moderation": "low",
            }
            
            # Set size - use detected API size or explicit size
            if preserve_size and api_size:
                data["size"] = api_size
            elif size != "auto":
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
            
            print(f"[gpt_image] Sending edit request with size={data.get('size', 'not set')}")
            
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
            
            # Process results
            urls = []
            for item in result.get("data", []):
                image_b64 = item.get("b64_json")
                if image_b64:
                    result_bytes = base64.b64decode(image_b64)
                    
                    # Remove padding to restore original dimensions exactly
                    if preserve_size and crop_box and original_size:
                        result_bytes = self._crop_to_original(result_bytes, crop_box, original_size, scale_factor)
                        print(f"[gpt_image] Restored to original size: {original_size[0]}x{original_size[1]}")
                    
                    url = self._upload_image(result_bytes, output_format)
                    urls.append(url)
            
            if not urls:
                return "Error: No images generated"
            
            return urls[0] if len(urls) == 1 else " | ".join(urls)
            
        finally:
            # Clean up
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
