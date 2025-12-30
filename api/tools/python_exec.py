"""
Python execution tool using Firecracker microVM.

Provides sandboxed Python code execution in an isolated VM with no network access.
Uses vsock for host-VM communication.
"""

import socket
import json
import base64
import os
import subprocess
import tempfile
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple
from .base import Tool
from api.utils.botbin import upload_to_botbin
from api.utils.output import log_info, log_error, log_warning, log_success


class PythonExecTool(Tool):
    """Python code execution tool using Firecracker microVM."""
    
    # Vsock configuration
    VSOCK_CID = 3  # Guest CID configured in start-vm.sh
    VSOCK_PORT = 5000
    
    # Timeout for execution (3 minutes)
    EXECUTION_TIMEOUT = 180
    
    def __init__(self, timeout: int = 180):
        """
        Initialize Python execution tool.
        
        Args:
            timeout: Max execution time in seconds (default 180 = 3 minutes)
        """
        self.timeout = timeout
        self.fc_dir = Path(__file__).parent.parent.parent / "scripts" / "firecracker"
        self.vsock_path = self.fc_dir / "vsock.sock"
        
    @property
    def name(self) -> str:
        return "python_exec"
    
    def get_definition(self) -> Dict[str, Any]:
        """Get tool definition for OpenAI API."""
        return {
            "type": "function",
            "name": "python_exec",
            "description": """Execute Python code in a secure sandboxed environment.

Features:
- Full Python 3.12 with scientific/visualization libraries
- matplotlib, numpy, pandas, Pillow for data/images
- graphviz, diagrams, networkx for diagrams
- seaborn, plotly for charts
- scipy, sympy for math

The sandbox has NO internet access for security.

Use for:
- Calculations and data analysis
- Generating charts, plots, diagrams
- Image manipulation
- Mathematical computations

Output files (images, etc.) are automatically uploaded and returned as URLs.
Save any files you want to return to the current directory.""",
            "parameters": {
                "type": "object",
                "properties": {
                    "code": {
                        "type": "string",
                        "description": "Python code to execute. Use print() for output. Save files to current directory for retrieval."
                    }
                },
                "required": ["code"],
                "additionalProperties": False
            }
        }
    
    def _is_vm_running(self) -> bool:
        """Check if the Firecracker VM is running."""
        pid_file = self.fc_dir / "firecracker.pid"
        if not pid_file.exists():
            return False
        try:
            pid = int(pid_file.read_text().strip())
            os.kill(pid, 0)
            return True
        except PermissionError:
            # Process exists but owned by root - that's fine, VM is running
            return True
        except (ProcessLookupError, ValueError):
            return False
    

    def _start_vm(self):
        proc = subprocess.Popen(
            ["sudo", "./start-vm.sh"],
            cwd=str(self.fc_dir),
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
        )

        for line in proc.stdout:
            if "localhost console" in line:
                log_info("VM booted successfully")
                return  # VM has booted
            #log_info(line.strip())

        raise RuntimeError("VM exited without booting")


    def _stop_vm(self):
        """Stop the Firecracker VM."""
        subprocess.run(["sudo", "./stop-vm.sh"], cwd=str(self.fc_dir), check=True)

    def _execute_via_vsock(self, code: str) -> Tuple[str, str, Dict[str, bytes]]:
        """
        Execute Python code via vsock connection to VM.
        
        Firecracker vsock protocol:
        1. Connect to Unix socket
        2. Send "CONNECT <port>\n"
        3. Receive "OK <port>\n"
        4. Then communicate normally
        
        Returns:
            Tuple of (stdout, stderr, dict of filename->bytes for output files)
        """
        if not self.vsock_path.exists():
            return "", "Error: vsock.sock not found. Is the VM running?", {}
        
        try:
            # Debug logging
            log_info(f"Vsock path: {self.vsock_path} (exists: {self.vsock_path.exists()})")
            
            # Connect to vsock via Unix socket
            sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            sock.settimeout(self.timeout + 10)
            
            log_info(f"Connecting to {self.vsock_path}...")
            sock.connect(str(self.vsock_path))
            
            # Firecracker vsock handshake: send CONNECT <port>\n
            connect_cmd = f"CONNECT {self.VSOCK_PORT}\n"
            sock.sendall(connect_cmd.encode("utf-8"))
            
            # Wait for OK response
            ok_response = b""
            while b"\n" not in ok_response:
                chunk = sock.recv(256)
                if not chunk:
                    break
                ok_response += chunk
            
            ok_str = ok_response.decode("utf-8").strip()
            if not ok_str.startswith("OK"):
                sock.close()
                return "", f"Error: vsock handshake failed: {ok_str}", {}
            
            log_info(f"Vsock connected: {ok_str}")
            
            # Now send the actual request
            request = json.dumps({"code": code}) + "\n\n"
            sock.sendall(request.encode("utf-8"))
            
            # Receive response
            data = b""
            while True:
                chunk = sock.recv(65536)
                if not chunk:
                    break
                data += chunk
                if b"\n\n" in data:
                    break
            
            sock.close()
            
            # Parse response
            response = json.loads(data.decode("utf-8").strip())
            stdout = response.get("stdout", "")
            stderr = response.get("stderr", "")
            files_b64 = response.get("files", {})
            
            # Decode base64 files
            files = {}
            for filename, b64data in files_b64.items():
                files[filename] = base64.b64decode(b64data)
            
            return stdout, stderr, files
            
        except socket.timeout:
            return "", f"Error: Execution timed out after {self.timeout} seconds", {}
        except ConnectionRefusedError:
            return "", "Error: VM vsock server not ready. Try again in a moment.", {}
        except Exception as e:
            log_error(f"Vsock execution failed: {e}")
            return "", f"Error: {str(e)}", {}
    
    def _execute_local_fallback(self, code: str) -> Tuple[str, str, Dict[str, bytes]]:
        """
        Fallback local execution for when VM is not running.
        
        WARNING: This is NOT sandboxed! Only for development.
        """
        log_warning("Firecracker VM not running - using LOCAL execution (NOT SANDBOXED)")
        
        work_dir = Path(tempfile.mkdtemp(prefix="pyexec_"))
        
        try:
            result = subprocess.run(
                ["python3", "-c", code],
                capture_output=True,
                text=True,
                timeout=self.timeout,
                cwd=str(work_dir)
            )
            
            stdout = result.stdout
            stderr = result.stderr
            
            # Collect output files
            files = {}
            for f in work_dir.iterdir():
                if f.is_file():
                    files[f.name] = f.read_bytes()
            
            return stdout, stderr, files
            
        except subprocess.TimeoutExpired:
            return "", f"Error: Execution timed out after {self.timeout} seconds", {}
        except Exception as e:
            return "", f"Error: {str(e)}", {}
    
    def execute(self, code: str, **kwargs) -> str:
        """
        Execute Python code.
        
        Args:
            code: Python code to execute
            
        Returns:
            Execution results - text for simple output, URLs for files
        """
        log_info(f"Executing Python code ({len(code)} chars)")
        
        vm_was_started = False
        
        try:
            # Start VM if not running
            if not self._is_vm_running():
                log_info("VM not running, starting with sudo...")
                try:
                    self._start_vm()
                    vm_was_started = True
                except Exception as e:
                    log_error(f"Failed to start VM: {e}")
                    return f"Error: Failed to start VM: {e}"
            
            # Execute code via vsock
            stdout, stderr, files = self._execute_via_vsock(code)
            
            # Check for errors
            if stderr and not stdout and not files:
                return f"Error:\n{stderr}"
            
            # Handle output files (images, etc.)
            uploaded_urls = []
            for filename, content in files.items():
                try:
                    url = upload_to_botbin(content, filename)
                    if url and not url.startswith("Error"):
                        uploaded_urls.append(f"{filename}: {url}")
                        log_success(f"Uploaded {filename} to botbin")
                except Exception as e:
                    log_error(f"Failed to upload {filename}: {e}")
            
            # Build result
            result_parts = []
            
            if stdout.strip():
                result_parts.append(f"Output:\n{stdout.strip()}")
            
            if stderr.strip():
                result_parts.append(f"Warnings:\n{stderr.strip()}")
            
            if uploaded_urls:
                result_parts.append("Files:\n" + "\n".join(uploaded_urls))
            
            combined = "\n\n".join(result_parts) if result_parts else "Code executed successfully (no output)"
            
            # If output is small enough, return directly
            if len(combined) < 500 and not uploaded_urls:
                return combined
            
            # Otherwise upload the full output + code to botbin
            full_output = f"=== Code ===\n{code}\n\n=== Result ===\n{combined}"
            paste_url = upload_to_botbin(full_output.encode(), "python_execution.txt")
            
            if uploaded_urls:
                urls_summary = ", ".join([u.split(": ")[1] for u in uploaded_urls])
                return f"Execution complete: {paste_url}\nFiles: {urls_summary}"
            else:
                return f"Execution complete: {paste_url}"
        
        finally:
            # Always stop VM after execution
            if vm_was_started or self._is_vm_running():
                log_info("Stopping VM with sudo...")
                try:
                    self._stop_vm()
                    log_success("VM stopped successfully")
                except Exception as e:
                    log_error(f"Failed to stop VM: {e}")