"""
Argument validation utilities for the Lolo Python API.

Validates command arguments against their schema definitions.
"""

from typing import List, Dict, Any, Tuple, Optional
from api.router import ArgumentSchema


class ValidationError(Exception):
    """
    Exception raised when argument validation fails.
    """
    def __init__(self, errors: List[str]):
        self.errors = errors
        super().__init__("; ".join(errors))


def validate_arguments(args: List[str], schema: List[ArgumentSchema]) -> Tuple[bool, List[str]]:
    """
    Validate command arguments against their schema.
    
    Args:
        args: List of argument strings from the command
        schema: List of ArgumentSchema defining expected arguments
        
    Returns:
        Tuple of (is_valid, error_messages)
        - is_valid: True if validation passed, False otherwise
        - error_messages: List of validation error messages (empty if valid)
    """
    errors = []
    
    # Check required arguments
    required_args = [arg for arg in schema if arg.required]
    if len(args) < len(required_args):
        missing = required_args[len(args):]
        for arg in missing:
            errors.append(f"Missing required argument: {arg.name} ({arg.description})")
    
    # Validate each provided argument
    for i, arg_schema in enumerate(schema):
        if i >= len(args):
            # No more args provided
            if arg_schema.required:
                # Already handled above
                pass
            break
        
        arg_value = args[i]
        
        # Type validation
        if arg_schema.type == "int":
            try:
                int(arg_value)
            except ValueError:
                errors.append(f"Argument '{arg_schema.name}' must be an integer, got: {arg_value}")
        
        elif arg_schema.type == "float":
            try:
                float(arg_value)
            except ValueError:
                errors.append(f"Argument '{arg_schema.name}' must be a number, got: {arg_value}")
        
        elif arg_schema.type == "user":
            # User should be a valid IRC nickname (alphanumeric, _, -, [, ], {, }, |, \, ^, `)
            if not arg_value or not all(c.isalnum() or c in "_-[]{}|\\^`" for c in arg_value):
                errors.append(f"Argument '{arg_schema.name}' must be a valid IRC nickname")
        
        elif arg_schema.type == "channel":
            # Channel should start with # or &
            if not arg_value.startswith(("#", "&")):
                errors.append(f"Argument '{arg_schema.name}' must be a valid channel name (starting with # or &)")
        
        elif arg_schema.type == "string":
            # String is always valid, but check if empty when required
            if arg_schema.required and not arg_value.strip():
                errors.append(f"Argument '{arg_schema.name}' cannot be empty")
        
        # Add more type validations as needed
    
    return (len(errors) == 0, errors)


def format_validation_errors(errors: List[str]) -> str:
    """
    Format validation errors into a user-friendly message.
    
    Args:
        errors: List of validation error messages
        
    Returns:
        Formatted error message string
    """
    if not errors:
        return ""
    
    if len(errors) == 1:
        return f"Validation error: {errors[0]}"
    
    error_list = "\n".join(f"  - {error}" for error in errors)
    return f"Validation errors:\n{error_list}"
