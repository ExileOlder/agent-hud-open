"""Build the portable helper with the Windows SDK, keeping Go out of the shim."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

source, output = (Path(p).resolve() for p in sys.argv[1:3])
vswhere = Path(os.environ["ProgramFiles(x86)"]) / "Microsoft Visual Studio/Installer/vswhere.exe"
vs = subprocess.check_output(
    [vswhere, "-latest", "-products", "*", "-requires",
     "Microsoft.VisualStudio.Component.VC.Tools.x86.x64", "-property", "installationPath", "-utf8"],
    encoding="utf-8", errors="replace",
).strip()
if not vs:
    sys.exit("Building the Windows portable package requires Visual Studio C++ build tools.")
setup = Path(vs) / "Common7/Tools/VsDevCmd.bat"

with tempfile.TemporaryDirectory() as temp:
    script = Path(temp) / "compile.cmd"
    script.write_text(
        "@echo off\r\n"
        'call "%BALANCE_MONITOR_MSVC%" -arch=amd64 -host_arch=amd64 >nul\r\n'
        "if errorlevel 1 exit /b 1\r\n"
        'cl /nologo /O1 /GS- /Zl /W4 /WX /c "%BALANCE_MONITOR_SOURCE%" /Fomonitor.obj\r\n'
        "if errorlevel 1 exit /b 1\r\n"
        'link /nologo /subsystem:console /entry:balance_monitor_entry /nodefaultlib '
        '/out:"%BALANCE_MONITOR_OUTPUT%" monitor.obj kernel32.lib\r\n',
        encoding="ascii", newline="",
    )
    subprocess.run(
        ["cmd.exe", "/d", "/v:off", "/c", "compile.cmd"], cwd=temp, check=True,
        env=dict(os.environ, BALANCE_MONITOR_MSVC=str(setup),
                 BALANCE_MONITOR_SOURCE=str(source), BALANCE_MONITOR_OUTPUT=str(output)),
    )
