"""
Flux image creation tool implementation.

Provides image generation using BFL's Flux API.
"""

import os
import time
import requests
import tempfile
from typing import Any, Dict, Optional
from .base import Tool


class FluxCreateTool(Tool):
    """Image creation tool using Flux API."""
    
    def __init__(self):
        """Initialize Flux create tool."""
        self.api_key = os.environ.get("BFL_API_KEY")
        self.freeimage_api_key = os.environ.get("FREEIMAGE_API_KEY")
        self.upload_url = "https://freeimage.host/api/1/upload"
    
    @property
    def name(self) -> str:
        return "flux_create_image"
    
    def get_definition(self) -> Dict[str, Any]:
        """
        Get tool definition for OpenAI API.
        
        Returns:
            Tool definition dict for custom function
        """
        return {
            "type": "function",
            "name": "flux_create_image",
            "description": "Generate images from text prompts using Flux AI. Returns a URL to the generated image. Default size is 1024x1024, default model is flux-2-pro (fast). Common sizes: 1024x1024 (square), 1920x1088 (widescreen 16:9), 1088x1920 (portrait 9:16), 2048x1024 (ultrawide 2:1). Dimensions must be multiples of 16.",
            "parameters": {
                "type": "object",
                "properties": {
                    "prompt": {
                        "type": "string",
                        "description": "Text description of the image to generate"
                    },
                    "width": {
                        "type": "integer",
                        "description": "Image width in pixels (must be multiple of 16, min 64, max 4096). Default: 1024"
                    },
                    "height": {
                        "type": "integer",
                        "description": "Image height in pixels (must be multiple of 16, min 64, max 4096). Default: 1024"
                    },
                    "model": {
                        "type": "string",
                        "enum": ["flux-2-pro", "flux-2-flex"],
                        "description": "Model to use: flux-2-pro (fast, <10s) or flux-2-flex (higher quality, slower). Default: flux-2-pro"
                    },
                    "output_format": {
                        "type": "string",
                        "enum": ["jpeg", "png"],
                        "description": "Output image format. Default: jpeg"
                    }
                },
                "required": ["prompt"],
                "additionalProperties": False
            }
        }
    
    def execute(self, prompt: str, width: int = 1024, height: int = 1024, model: str = "flux-2-pro", 
                output_format: Optional[str] = None, **kwargs) -> str:
        """
        Execute image generation.
        
        Args:
            prompt: Text description of image
            width: Image width in pixels (default: 1024)
            height: Image height in pixels (default: 1024)
            model: Model to use (default: flux-2-pro)
            output_format: Output format jpeg/png (optional, default: jpeg)
            **kwargs: Additional optional parameters
            
        Returns:
            URL to the generated image or error message
        """
        if not self.api_key:
            return "Error: BFL_API_KEY not configured"
        
        # Validate dimensions
        if width % 16 != 0 or height % 16 != 0:
            return f"Error: Width and height must be multiples of 16. Got {width}x{height}"
        if width < 64 or height < 64 or width > 4096 or height > 4096:
            return f"Error: Dimensions must be between 64 and 4096. Got {width}x{height}"
        
        # Validate model
        if model not in ["flux-2-pro", "flux-2-flex"]:
            return f"Error: Model must be flux-2-pro or flux-2-flex. Got {model}"
        
        # Validate optional parameters
        if output_format is not None and output_format not in ["jpeg", "png"]:
            return f"Error: output_format must be jpeg or png. Got {output_format}"
        
        try:
            # Build request payload
            payload = {
                "prompt": prompt,
                "width": width,
                "height": height,
                "safety_tolerance": 5,
                "output_format": output_format if output_format is not None else "jpeg"
            }
            
            # Create generation request
            response = requests.post(
                f"https://api.bfl.ai/v1/{model}",
                headers={
                    "accept": "application/json",
                    "x-key": self.api_key,
                    "Content-Type": "application/json"
                },
                json=payload
            )
            
            # Check for errors
            if response.status_code != 200:
                error_detail = response.text
                return f"Error: {response.status_code} {error_detail}"
            
            result = response.json()
            
            request_id = result["id"]
            polling_url = result["polling_url"]
            
            # Poll for result
            max_attempts = 60
            for _ in range(max_attempts):
                time.sleep(1)
                poll_response = requests.get(
                    polling_url,
                    headers={
                        "accept": "application/json",
                        "x-key": self.api_key
                    }
                )
                poll_response.raise_for_status()
                poll_result = poll_response.json()
                
                if poll_result["status"] == "Ready":
                    image_url = poll_result["result"]["sample"]
                    
                    # Download image
                    img_response = requests.get(image_url)
                    img_response.raise_for_status()
                    
                    # Upload to paste
                    with tempfile.NamedTemporaryFile(suffix=".jpeg", delete=False) as tmp:
                        tmp.write(img_response.content)
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
                            return f"Error: Failed to upload image - {upload_response.status_code} {upload_response.text}"
                        
                        upload_result = upload_response.json()
                        
                        if upload_result.get("status_code") != 200:
                            error_msg = upload_result.get("error", {}).get("message", "Unknown error")
                            return f"Error: Image upload failed - {error_msg}"
                        
                        # Get the direct image URL
                        image_url = upload_result["image"]["url"]
                        return image_url
                    finally:
                        os.unlink(tmp_path)
                
                elif poll_result["status"] in ["Error", "Failed"]:
                    return f"Error: Image generation failed - {poll_result.get('error', 'Unknown error')}"
            
            return "Error: Image generation timed out"
            
        except Exception as e:
            return f"Error: {str(e)}"
