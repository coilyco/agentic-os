@echo off
REM docker-credential-forgejo-ssm.cmd - Windows Docker credential entry point.
REM The extensionless Bash helper sits alongside this wrapper.

"C:\Program Files\Git\usr\bin\bash.exe" "%~dp0docker-credential-forgejo-ssm" %*
