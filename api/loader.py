"""
Dynamic command loader for the Lolo Python API.

Loads command modules from the api/commands/ directory and registers them
for execution. New commands can be added by simply creating a new file in
the commands directory without modifying core routing logic.
"""

import os
import importlib
import inspect
from typing import Dict, Callable, Optional, Any
from pathlib import Path
from rich.console import Console

from api.router import CommandRequest, CommandResponse, CommandMetadata

console = Console()


class CommandLoader:
    """
    Dynamically loads and manages command modules.
    """
    
    def __init__(self):
        self.commands: Dict[str, Callable] = {}
        self.metadata: Dict[str, CommandMetadata] = {}
        self.commands_dir = Path(__file__).parent / "commands"
    
    def load_commands(self) -> None:
        """
        Load all command modules from the commands directory.
        
        Each command module should export a handle() function that takes
        a CommandRequest and returns a CommandResponse.
        
        Optionally, modules can export a get_metadata() function that returns
        CommandMetadata for the command.
        """
        console.print(f"[cyan]Loading commands from {self.commands_dir}...[/cyan]")
        
        if not self.commands_dir.exists():
            console.print(f"[yellow]Warning: Commands directory not found, creating it[/yellow]")
            self.commands_dir.mkdir(parents=True, exist_ok=True)
            return
        
        # Find all Python files in commands directory
        for file_path in self.commands_dir.glob("*.py"):
            if file_path.name.startswith("_"):
                continue  # Skip __init__.py and private modules
            
            module_name = file_path.stem
            self._load_command_module(module_name)
    
    def _load_command_module(self, module_name: str) -> None:
        """
        Load a single command module.
        
        Args:
            module_name: Name of the module (without .py extension)
        """
        try:
            # Import the module
            module = importlib.import_module(f"api.commands.{module_name}")
            
            # Check if module has a handle function
            if not hasattr(module, "handle"):
                console.print(f"[yellow]![/yellow] Module {module_name} has no handle() function, skipping")
                return
            
            handle_func = getattr(module, "handle")
            
            # Verify function signature
            sig = inspect.signature(handle_func)
            if len(sig.parameters) != 1:
                console.print(f"[yellow]![/yellow] Module {module_name} handle() should take exactly 1 parameter, skipping")
                return
            
            # Register command
            self.commands[module_name] = handle_func
            
            # Load metadata if available
            if hasattr(module, "get_metadata"):
                try:
                    metadata = module.get_metadata()
                    self.metadata[module_name] = metadata
                    console.print(f"[green]✓[/green] Loaded command: [bold]{module_name}[/bold] "
                                  f"(permission: {metadata.required_permission})")
                except Exception as e:
                    console.print(f"[yellow]![/yellow] Failed to load metadata for {module_name}: {e}")
                    # Create default metadata
                    self.metadata[module_name] = CommandMetadata(
                        name=module_name,
                        help_text=f"Command: {module_name}",
                        required_permission="any"
                    )
            else:
                # Create default metadata
                self.metadata[module_name] = CommandMetadata(
                    name=module_name,
                    help_text=f"Command: {module_name}",
                    required_permission="any"
                )
                console.print(f"[green]✓[/green] Loaded command: [bold]{module_name}[/bold] (no metadata)")
            
        except Exception as e:
            console.print(f"[red]✗[/red] Failed to load command {module_name}: {e}")
    
    def get_command(self, command_name: str) -> Optional[Callable]:
        """
        Get a command handler by name.
        
        Args:
            command_name: Name of the command
            
        Returns:
            Command handler function or None if not found
        """
        return self.commands.get(command_name)
    
    def get_metadata(self, command_name: str) -> Optional[CommandMetadata]:
        """
        Get metadata for a command.
        
        Args:
            command_name: Name of the command
            
        Returns:
            CommandMetadata or None if not found
        """
        return self.metadata.get(command_name)
    
    def reload_commands(self) -> None:
        """
        Reload all command modules.
        
        Useful for hot-reloading during development.
        """
        console.print("[cyan]Reloading all commands...[/cyan]")
        self.commands.clear()
        self.metadata.clear()
        
        # Reload modules
        for module_name in list(self.commands.keys()):
            try:
                module = importlib.import_module(f"api.commands.{module_name}")
                importlib.reload(module)
            except Exception as e:
                console.print(f"[red]✗[/red] Failed to reload {module_name}: {e}")
        
        self.load_commands()
