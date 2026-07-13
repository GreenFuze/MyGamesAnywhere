#define AppName "MGA Client"
#define AppPublisher "GreenFuze"
#define AppExeName "mga-client.exe"

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

[Icons]
Name: "{group}\MGA Client Status"; Filename: "{app}\{#AppExeName}"; Parameters: "status"
Name: "{group}\MGA Client Doctor"; Filename: "{app}\{#AppExeName}"; Parameters: "doctor"

[Registry]
Root: HKCU; Subkey: "Software\Classes\mga"; ValueType: string; ValueName: ""; ValueData: "URL:MGA Protocol"; Flags: uninsdeletekey
Root: HKCU; Subkey: "Software\Classes\mga"; ValueType: string; ValueName: "URL Protocol"; ValueData: ""
Root: HKCU; Subkey: "Software\Classes\mga\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\{#AppExeName},0"
Root: HKCU; Subkey: "Software\Classes\mga\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\{#AppExeName}"" protocol ""%1"""
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "MGA Client"; ValueData: """{app}\{#AppExeName}"" agent"; Flags: uninsdeletevalue
