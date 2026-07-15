#define AppName "MGA Client"
#define AppPublisher "GreenFuze"
#define AppExeName "mga-client.exe"
#define AgentExeName "mga-client-agent.exe"
#define NoticesName "THIRD_PARTY_NOTICES.md"

[Setup]
AppId={{8BD5321B-C2BA-45C8-91BA-B22F1945964A}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
DefaultDirName={localappdata}\Programs\MGA Client
DefaultGroupName=MGA Client
PrivilegesRequired=lowest
OutputDir={#OutputDir}
OutputBaseFilename=mga-client-windows-amd64-installer
Compression=lzma2
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\{#AppExeName}
WizardStyle=modern

[Files]
Source: "{#ClientExe}"; DestDir: "{app}"; DestName: "{#AppExeName}"; Flags: ignoreversion
Source: "{#AgentExe}"; DestDir: "{app}"; DestName: "{#AgentExeName}"; Flags: ignoreversion
Source: "{#NoticesFile}"; DestDir: "{app}"; DestName: "{#NoticesName}"; Flags: ignoreversion

[Icons]
Name: "{group}\MGA Client Status"; Filename: "{app}\{#AppExeName}"; Parameters: "status"
Name: "{group}\MGA Client Doctor"; Filename: "{app}\{#AppExeName}"; Parameters: "doctor"

[Registry]
Root: HKCU; Subkey: "Software\Classes\mga"; ValueType: string; ValueName: ""; ValueData: "URL:MGA Protocol"; Flags: uninsdeletekey
Root: HKCU; Subkey: "Software\Classes\mga"; ValueType: string; ValueName: "URL Protocol"; ValueData: ""
Root: HKCU; Subkey: "Software\Classes\mga\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\{#AgentExeName},0"
Root: HKCU; Subkey: "Software\Classes\mga\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\{#AgentExeName}"" protocol ""%1"""
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "MGA Client"; ValueData: """{app}\{#AgentExeName}"" agent"; Flags: uninsdeletevalue

[Run]
Filename: "{app}\{#AgentExeName}"; Parameters: "agent"; Flags: nowait
