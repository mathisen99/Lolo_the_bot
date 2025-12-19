"""
Flux image editing tool implementation.

Provides image editing using BFL's Flux API.
"""

import os
import time
import requests
from typing import Any, Dict, Optional
from .base import Tool
from api.utils.botbin import upload_to_botbin


class FluxEditTool(Tool):
    """Image editing tool using Flux API."""
    
    def __init__(self):
        """Initialize Flux edit tool."""
        self.api_key = os.environ.get("BFL_API_KEY")
    
    @property
    def name(self) -> str:
        return "flux_edit_image"
    
    def get_definition(self) -> Dict[str, Any]:
        """
        Get tool definition for OpenAI API.
        
        Returns:
            Tool definition dict for custom function
        """
        return {
            "type": "function",
            "name": "flux_edit_image",
            "description": "Edit images using text prompts with Flux AI. Can download images from URLs shared in IRC. Returns a URL to the edited image. Default model is flux-2-pro (fast). By default matches input image dimensions. Dimensions must be multiples of 16.",
            "parameters": {
                "type": "object",
                "properties": {
                    "prompt": {
                        "type": "string",
                        "description": "Text description of the edit to apply"
                    },
                    "input_image_url": {
                        "type": "string",
                        "description": "URL of the image to edit (can be from IRC or any accessible URL)"
                    },
                    "width": {
                        "type": "integer",
                        "description": "Output width in pixels (must be multiple of 16, min 64, max 4096). If omitted, matches input image width"
                    },
                    "height": {
                        "type": "integer",
                        "description": "Output height in pixels (must be multiple of 16, min 64, max 4096). If omitted, matches input image height"
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
                "required": ["prompt", "input_image_url"],
                "additionalProperties": False
            }
        }
    
    def execute(self, prompt: str, input_image_url: str, width: Optional[int] = None, 
                height: Optional[int] = None, model: str = "flux-2-pro", 
                output_format: Optional[str] = None, **kwargs) -> str:
        """
        Execute image editing.
        
        Args:
            prompt: Text description of edit
            input_image_url: URL of image to edit
            width: Output width in pixels (optional, defaults to input image width)
            height: Output height in pixels (optional, defaults to input image height)
            model: Model to use (default: flux-2-pro)
            output_format: Output format jpeg/png (optional, default: jpeg)
            **kwargs: Additional optional parameters
            
        Returns:
            URL to the edited image or error message
        """
        if not self.api_key:
            return "Error: BFL_API_KEY not configured"
        
        # Validate dimensions if provided
        if width is not None:
            if width % 16 != 0:
                return f"Error: Width must be a multiple of 16. Got {width}"
            if width < 64 or width > 4096:
                return f"Error: Width must be between 64 and 4096. Got {width}"
        
        if height is not None:
            if height % 16 != 0:
                return f"Error: Height must be a multiple of 16. Got {height}"
            if height < 64 or height > 4096:
                return f"Error: Height must be between 64 and 4096. Got {height}"
        
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
                "input_image": input_image_url,
                "safety_tolerance": 5,
                "output_format": output_format if output_format is not None else "jpeg"
            }
            
            # Add optional dimensions if provided
            if width is not None:
                payload["width"] = width
            if height is not None:
                payload["height"] = height
            
            # Create edit request
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
                    
                    # Upload to botbin
                    fmt = output_format if output_format else "jpeg"
                    url = upload_to_botbin(img_response.content, f"image.{fmt}")
                    return url
                
                elif poll_result["status"] in ["Error", "Failed"]:
                    return f"Error: Image editing failed - {poll_result.get('error', 'Unknown error')}"
            
            return "Error: Image editing timed out"
            
        except Exception as e:
            return f"Error: {str(e)}"
