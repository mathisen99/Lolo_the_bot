"""
GPT Image tool implementation.

Provides image generation and editing using OpenAI's gpt-image-2 model.
"""

import base64
import io
import math
import os
import re
import tempfile
from typing import Any, Dict, List, Optional, Tuple

import requests
from PIL import Image

from .base import Tool
from api.ai.usage_tracker import calculate_multimodal_cost, extract_usage_from_image_result, log_usage
from api.utils.botbin import upload_to_botbin


class GPTImageTool(Tool):
    """Image generation and editing tool using OpenAI's gpt-image-2 model."""

    MODEL = "gpt-image-2"
    MAX_EDGE = 3840
    SIZE_MULTIPLE = 16
    MAX_ASPECT_RATIO = 3.0
    MIN_PIXELS = 655_360
    MAX_PIXELS = 8_294_400
    SIZE_PATTERN = re.compile(r"^(?P<width>\d+)x(?P<height>\d+)$")

    VALID_QUALITIES = ["low", "medium", "high", "auto"]
    VALID_FORMATS = ["png", "jpeg", "webp"]
    VALID_BACKGROUNDS = ["opaque", "auto"]
    VALID_MODERATION = ["auto", "low"]

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
            "description": """Generate or edit images using OpenAI's gpt-image-2 model.

CAPABILITIES:
- Generate high-quality images from text prompts
- Edit existing images or create new compositions from reference images
- Inpainting with masks
- Accurate text rendering for signs, posters, labels, diagrams
- Custom output size, quality, format, and compression

SIZE HANDLING:
- Use size="auto" to let the API choose automatically
- Or pass any valid WIDTHxHEIGHT resolution supported by GPT Image 2
- When editing a single image or masked image with size="auto", the tool preserves the original dimensions when possible

LIMITATION:
- gpt-image-2 does not support transparent backgrounds

USE THIS WHEN:
- User wants the best OpenAI image model
- User needs accurate text in images
- User wants high-quality edits or reference-image composition
- User asks for "GPT image" or "OpenAI image"

Returns a URL to the generated or edited image.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "prompt": {
                        "type": "string",
                        "description": "Text description of the image to generate, or the edit to apply."
                    },
                    "input_image_urls": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Optional input image URLs for editing or reference-guided generation. GPT Image 2 processes all input images at high fidelity."
                    },
                    "mask_url": {
                        "type": "string",
                        "description": "Optional mask image URL for inpainting. The mask is aligned to the first input image and will be converted to PNG with alpha if needed."
                    },
                    "size": {
                        "type": "string",
                        "description": "Output size. Use 'auto' or a valid WIDTHxHEIGHT string such as 1024x1024, 2048x1152, or 2160x3840. GPT Image 2 sizes must use 16px multiples, max edge 3840, max 3:1 aspect ratio, and 655360-8294400 total pixels."
                    },
                    "quality": {
                        "type": "string",
                        "enum": ["low", "medium", "high", "auto"],
                        "description": "Image quality. low is fastest, high is best quality. Default: auto"
                    },
                    "output_format": {
                        "type": "string",
                        "enum": ["png", "jpeg", "webp"],
                        "description": "Output format. Default: png"
                    },
                    "background": {
                        "type": "string",
                        "enum": ["opaque", "auto"],
                        "description": "Background type. GPT Image 2 supports opaque or auto backgrounds only."
                    },
                    "output_compression": {
                        "type": "integer",
                        "minimum": 0,
                        "maximum": 100,
                        "description": "Compression level for jpeg/webp output (0-100). Ignored for png."
                    },
                    "moderation": {
                        "type": "string",
                        "enum": ["auto", "low"],
                        "description": "Moderation strictness. low is less restrictive. Default: low"
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
        """Extract image dimensions from bytes using PIL."""
        try:
            img = Image.open(io.BytesIO(img_bytes))
            return img.size
        except Exception:
            return None

    def _parse_size(self, size: str) -> Optional[Tuple[int, int]]:
        """Parse WIDTHxHEIGHT strings."""
        match = self.SIZE_PATTERN.match(size.strip())
        if not match:
            return None
        return (int(match.group("width")), int(match.group("height")))

    def _size_to_string(self, width: int, height: int) -> str:
        """Convert size tuple to API string."""
        return f"{width}x{height}"

    def _is_valid_api_size(self, width: int, height: int) -> bool:
        """Validate GPT Image 2 output size constraints."""
        if width <= 0 or height <= 0:
            return False
        if width > self.MAX_EDGE or height > self.MAX_EDGE:
            return False
        if width % self.SIZE_MULTIPLE != 0 or height % self.SIZE_MULTIPLE != 0:
            return False

        short_edge = min(width, height)
        long_edge = max(width, height)
        if short_edge == 0 or (long_edge / short_edge) > self.MAX_ASPECT_RATIO:
            return False

        total_pixels = width * height
        return self.MIN_PIXELS <= total_pixels <= self.MAX_PIXELS

    def _validate_size(self, size: str) -> Optional[str]:
        """Return an error message if the requested size is invalid."""
        if size == "auto":
            return None

        dims = self._parse_size(size)
        if not dims:
            return "Error: Invalid size. Use 'auto' or a WIDTHxHEIGHT string like 1024x1024."

        width, height = dims
        if self._is_valid_api_size(width, height):
            return None

        return (
            "Error: Invalid size for gpt-image-2. Sizes must use 16px multiples, "
            "have each edge <= 3840, stay within a 3:1 aspect ratio, and total "
            "655360-8294400 pixels."
        )

    def _find_best_target_size(self, width: int, height: int) -> Tuple[str, Tuple[int, int]]:
        """
        Find the smallest valid GPT Image 2 size that preserves the source image as much
        as possible, using padding where possible and scaling only when required.
        """
        if self._is_valid_api_size(width, height):
            return (self._size_to_string(width, height), (width, height))

        target_ratio = width / height if height else 1.0
        best_size: Optional[Tuple[int, int]] = None
        best_score: Optional[Tuple[float, int, int, float]] = None

        for target_width in range(self.SIZE_MULTIPLE, self.MAX_EDGE + self.SIZE_MULTIPLE, self.SIZE_MULTIPLE):
            for target_height in range(self.SIZE_MULTIPLE, self.MAX_EDGE + self.SIZE_MULTIPLE, self.SIZE_MULTIPLE):
                total_pixels = target_width * target_height
                if total_pixels < self.MIN_PIXELS or total_pixels > self.MAX_PIXELS:
                    continue

                short_edge = min(target_width, target_height)
                long_edge = max(target_width, target_height)
                if short_edge == 0 or (long_edge / short_edge) > self.MAX_ASPECT_RATIO:
                    continue

                scale = min(1.0, target_width / width, target_height / height)
                scaled_width = max(1, min(target_width, math.floor((width * scale) + 1e-6)))
                scaled_height = max(1, min(target_height, math.floor((height * scale) + 1e-6)))
                padding_area = total_pixels - (scaled_width * scaled_height)
                ratio_diff = abs((target_width / target_height) - target_ratio)
                score = (-scale, total_pixels, padding_area, ratio_diff)

                if best_score is None or score < best_score:
                    best_score = score
                    best_size = (target_width, target_height)

        if not best_size:
            return ("1024x1024", (1024, 1024))

        return (self._size_to_string(best_size[0], best_size[1]), best_size)

    def _pad_image_to_size(
        self,
        img_bytes: bytes,
        target_size: Tuple[int, int]
    ) -> Tuple[bytes, Tuple[int, int, int, int], Tuple[int, int], float]:
        """
        Pad image to target size, centering the original. Scales down only if needed.
        Returns (padded_image_bytes, crop_box, original_size, scale_factor).
        """
        img = Image.open(io.BytesIO(img_bytes))
        orig_width, orig_height = img.size
        target_width, target_height = target_size

        scale = min(1.0, target_width / orig_width, target_height / orig_height)
        if scale < 1.0:
            new_width = max(1, math.floor((orig_width * scale) + 1e-6))
            new_height = max(1, math.floor((orig_height * scale) + 1e-6))
            img = img.resize((new_width, new_height), Image.Resampling.LANCZOS)
            print(
                f"[gpt_image] Scaled input from {orig_width}x{orig_height} "
                f"to {new_width}x{new_height} to fit GPT Image 2 limits"
            )
            scaled_width, scaled_height = new_width, new_height
        else:
            scaled_width, scaled_height = orig_width, orig_height

        left = (target_width - scaled_width) // 2
        top = (target_height - scaled_height) // 2
        right = left + scaled_width
        bottom = top + scaled_height

        uses_alpha = "A" in img.getbands() or "transparency" in img.info
        if uses_alpha:
            img = img.convert("RGBA")
            padded = Image.new("RGBA", (target_width, target_height), (255, 255, 255, 0))
            padded.paste(img, (left, top), img)
        else:
            if img.mode != "RGB":
                img = img.convert("RGB")
            padded = Image.new("RGB", (target_width, target_height), (255, 255, 255))
            padded.paste(img, (left, top))

        output = io.BytesIO()
        padded.save(output, format="PNG")
        return (output.getvalue(), (left, top, right, bottom), (orig_width, orig_height), scale)

    def _prepare_mask_bytes(
        self,
        mask_bytes: bytes,
        first_image_size: Tuple[int, int],
        target_size: Optional[Tuple[int, int]] = None
    ) -> bytes:
        """
        Normalize a mask to PNG RGBA, ensure it matches the first image size, and
        apply the same scale/padding transform when preserving size.
        """
        mask = Image.open(io.BytesIO(mask_bytes))
        if mask.size != first_image_size:
            print(
                f"[gpt_image] Resizing mask from {mask.size[0]}x{mask.size[1]} "
                f"to {first_image_size[0]}x{first_image_size[1]} to match input image"
            )
            mask = mask.resize(first_image_size, Image.Resampling.NEAREST)

        if "A" not in mask.getbands():
            grayscale = mask.convert("L")
            mask = grayscale.convert("RGBA")
            mask.putalpha(grayscale)
        else:
            mask = mask.convert("RGBA")

        if target_size:
            target_width, target_height = target_size
            orig_width, orig_height = mask.size
            scale = min(1.0, target_width / orig_width, target_height / orig_height)
            if scale < 1.0:
                new_width = max(1, math.floor((orig_width * scale) + 1e-6))
                new_height = max(1, math.floor((orig_height * scale) + 1e-6))
                mask = mask.resize((new_width, new_height), Image.Resampling.NEAREST)
            else:
                new_width, new_height = orig_width, orig_height

            left = (target_width - new_width) // 2
            top = (target_height - new_height) // 2
            canvas = Image.new("RGBA", target_size, (0, 0, 0, 0))
            canvas.paste(mask, (left, top), mask)
            mask = canvas

        output = io.BytesIO()
        mask.save(output, format="PNG")
        return output.getvalue()

    def _encode_image(
        self,
        img: Image.Image,
        output_format: str,
        output_compression: Optional[int]
    ) -> bytes:
        """Encode a PIL image using the requested output format."""
        output = io.BytesIO()

        if output_format == "png":
            if img.mode not in ("RGB", "RGBA"):
                img = img.convert("RGBA" if "A" in img.getbands() else "RGB")
            img.save(output, format="PNG")
            return output.getvalue()

        quality = None
        if output_compression is not None:
            quality = max(1, min(100, 100 - output_compression))

        if output_format == "jpeg":
            if img.mode != "RGB":
                img = img.convert("RGB")
            save_kwargs: Dict[str, Any] = {"format": "JPEG"}
            if quality is not None:
                save_kwargs["quality"] = quality
            img.save(output, **save_kwargs)
            return output.getvalue()

        if img.mode not in ("RGB", "RGBA"):
            img = img.convert("RGBA" if "A" in img.getbands() else "RGB")
        save_kwargs = {"format": "WEBP"}
        if quality is not None:
            save_kwargs["quality"] = quality
        img.save(output, **save_kwargs)
        return output.getvalue()

    def _crop_to_original(
        self,
        img_bytes: bytes,
        crop_box: Tuple[int, int, int, int],
        original_size: Tuple[int, int],
        scale: float,
        output_format: str,
        output_compression: Optional[int]
    ) -> bytes:
        """Remove padding from an edited image and restore original dimensions."""
        img = Image.open(io.BytesIO(img_bytes))
        cropped = img.crop(crop_box)

        if scale < 1.0:
            cropped = cropped.resize(original_size, Image.Resampling.LANCZOS)

        return self._encode_image(cropped, output_format, output_compression)

    def _upload_image(self, image_bytes: bytes, output_format: str = "png") -> str:
        """Upload image bytes to botbin.net and return URL."""
        return upload_to_botbin(image_bytes, f"image.{output_format}")

    def _log_api_usage(
        self,
        result: Dict[str, Any],
        request_id: Optional[str],
        nick: Optional[str],
        channel: Optional[str]
    ) -> None:
        """Record Image API token usage in the shared usage tracking table."""
        if not request_id or not nick:
            return

        usage = extract_usage_from_image_result(result)
        if usage["input_tokens"] <= 0 and usage["output_tokens"] <= 0:
            return

        cost_override = calculate_multimodal_cost(self.MODEL, result.get("usage"))
        usage_request_id = f"{request_id}:gpt_image"

        log_usage(
            request_id=usage_request_id,
            nick=nick,
            channel=channel,
            model=self.MODEL,
            input_tokens=usage["input_tokens"],
            cached_tokens=usage["cached_tokens"],
            output_tokens=usage["output_tokens"],
            cost_override=cost_override,
        )

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
        moderation: str = "low",
        n: int = 1,
        **kwargs
    ) -> str:
        """
        Generate or edit images using gpt-image-2.

        Args:
            prompt: Text description of image to generate or edit to apply
            input_image_urls: Optional list of input image URLs for editing
            mask_url: Optional mask URL for inpainting
            size: Output size ("auto" or WIDTHxHEIGHT)
            quality: Quality level (low, medium, high, auto)
            output_format: Output format (png, jpeg, webp)
            background: Background type (opaque, auto)
            output_compression: Compression for jpeg/webp (0-100)
            moderation: Moderation strictness (auto, low)
            n: Number of images to generate (1-4)

        Returns:
            URL(s) to the generated or edited image(s), or an error message
        """
        request_id = kwargs.get("_request_id")
        nick = kwargs.get("_nick")
        channel = kwargs.get("_channel")

        if not self.api_key:
            return "Error: OPENAI_API_KEY not configured"

        size_error = self._validate_size(size)
        if size_error:
            return size_error

        if quality not in self.VALID_QUALITIES:
            return f"Error: Invalid quality. Must be one of: {', '.join(self.VALID_QUALITIES)}"
        if output_format not in self.VALID_FORMATS:
            return f"Error: Invalid format. Must be one of: {', '.join(self.VALID_FORMATS)}"
        if background == "transparent":
            return "Error: gpt-image-2 does not support transparent backgrounds"
        if background not in self.VALID_BACKGROUNDS:
            return f"Error: Invalid background. Must be one of: {', '.join(self.VALID_BACKGROUNDS)}"
        if moderation not in self.VALID_MODERATION:
            return f"Error: Invalid moderation. Must be one of: {', '.join(self.VALID_MODERATION)}"
        if n < 1 or n > 4:
            return "Error: n must be between 1 and 4"

        if mask_url and not input_image_urls:
            return "Error: mask_url requires at least one input image"

        if output_compression is not None:
            if output_format == "png":
                output_compression = None
            elif output_compression < 0 or output_compression > 100:
                return "Error: output_compression must be between 0 and 100"

        try:
            headers = {
                "Authorization": f"Bearer {self.api_key}",
            }

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
                    moderation=moderation,
                    n=n,
                    headers=headers,
                    request_id=request_id,
                    nick=nick,
                    channel=channel,
                )

            return self._generate_image(
                prompt=prompt,
                size=size,
                quality=quality,
                output_format=output_format,
                background=background,
                output_compression=output_compression,
                moderation=moderation,
                n=n,
                headers=headers,
                request_id=request_id,
                nick=nick,
                channel=channel,
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
        moderation: str,
        n: int,
        headers: Dict[str, str],
        request_id: Optional[str],
        nick: Optional[str],
        channel: Optional[str],
    ) -> str:
        """Generate a new image from prompt."""
        payload: Dict[str, Any] = {
            "model": self.MODEL,
            "prompt": prompt,
            "n": n,
            "moderation": moderation,
        }

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
            timeout=180
        )

        if response.status_code != 200:
            return f"Error: {response.status_code} - {response.text}"

        result = response.json()
        self._log_api_usage(result, request_id, nick, channel)
        urls = []
        for item in result.get("data", []):
            image_b64 = item.get("b64_json")
            if not image_b64:
                continue
            image_bytes = base64.b64decode(image_b64)
            urls.append(self._upload_image(image_bytes, output_format))

        if not urls:
            return "Error: No images generated"

        return urls[0] if len(urls) == 1 else " | ".join(urls)

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
        moderation: str,
        n: int,
        headers: Dict[str, str],
        request_id: Optional[str],
        nick: Optional[str],
        channel: Optional[str],
    ) -> str:
        """Edit images with GPT Image 2, preserving size where appropriate."""
        image_files = []
        temp_files = []
        mask_file = None

        first_image_size: Optional[Tuple[int, int]] = None
        crop_box: Optional[Tuple[int, int, int, int]] = None
        scale_factor = 1.0
        api_size: Optional[str] = None
        target_size: Optional[Tuple[int, int]] = None

        preserve_size = size == "auto" and (mask_url is not None or len(input_image_urls) == 1)

        try:
            for i, url in enumerate(input_image_urls):
                img_bytes = self._download_image(url)

                if i == 0:
                    dims = self._get_image_dimensions(img_bytes)
                    if dims:
                        first_image_size = dims
                    elif preserve_size:
                        preserve_size = False
                        print("[gpt_image] Failed to detect first image size, falling back to API auto sizing")

                    if preserve_size and dims:
                        api_size, target_size = self._find_best_target_size(dims[0], dims[1])
                        if target_size != dims:
                            print(
                                f"[gpt_image] Preserving edit size by fitting {dims[0]}x{dims[1]} "
                                f"into API canvas {api_size}"
                            )
                        else:
                            print(f"[gpt_image] Using original edit size {api_size}")

                        img_bytes, crop_box, first_image_size, scale_factor = self._pad_image_to_size(
                            img_bytes, target_size
                        )

                tmp = tempfile.NamedTemporaryFile(suffix=".png", delete=False)
                tmp.write(img_bytes)
                tmp.close()
                temp_files.append(tmp.name)
                image_files.append(("image[]", (f"image{i}.png", open(tmp.name, "rb"))))

            if mask_url:
                if not first_image_size:
                    return "Error: Unable to determine first image size for mask alignment"

                mask_bytes = self._download_image(mask_url)
                mask_bytes = self._prepare_mask_bytes(
                    mask_bytes=mask_bytes,
                    first_image_size=first_image_size,
                    target_size=target_size if preserve_size else None
                )

                mask_tmp = tempfile.NamedTemporaryFile(suffix=".png", delete=False)
                mask_tmp.write(mask_bytes)
                mask_tmp.close()
                temp_files.append(mask_tmp.name)
                mask_file = ("mask", ("mask.png", open(mask_tmp.name, "rb")))

            data = {
                "model": self.MODEL,
                "prompt": prompt,
                "n": str(n),
                "moderation": moderation,
            }

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

            files = list(image_files)
            if mask_file:
                files.append(mask_file)

            print(f"[gpt_image] Sending edit request with size={data.get('size', 'auto')}")
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
            self._log_api_usage(result, request_id, nick, channel)
            urls = []
            for item in result.get("data", []):
                image_b64 = item.get("b64_json")
                if not image_b64:
                    continue

                result_bytes = base64.b64decode(image_b64)
                needs_restore = (
                    preserve_size and
                    crop_box is not None and
                    first_image_size is not None and
                    target_size is not None and
                    (crop_box != (0, 0, target_size[0], target_size[1]) or scale_factor < 1.0)
                )
                if needs_restore:
                    result_bytes = self._crop_to_original(
                        result_bytes,
                        crop_box,
                        first_image_size,
                        scale_factor,
                        output_format,
                        output_compression
                    )
                    print(f"[gpt_image] Restored edited image to {first_image_size[0]}x{first_image_size[1]}")

                urls.append(self._upload_image(result_bytes, output_format))

            if not urls:
                return "Error: No images generated"

            return urls[0] if len(urls) == 1 else " | ".join(urls)

        finally:
            for file_entry in image_files:
                try:
                    file_entry[1][1].close()
                except Exception:
                    pass

            if mask_file:
                try:
                    mask_file[1][1].close()
                except Exception:
                    pass

            for tmp_path in temp_files:
                try:
                    os.unlink(tmp_path)
                except Exception:
                    pass
