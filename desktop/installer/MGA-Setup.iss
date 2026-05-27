; MGA Desktop Client — Inno Setup script
; Builds a user-scope installer (no UAC elevation).
; Requires Inno Setup 6.x (https://jrsoftware.org/isinfo.php)
;
; Usage:
;   iscc.exe MGA-Setup.iss
;   Or via build script: desktop\scripts\build-installer.ps1
;
; Output: desktop\installer\Output\MGA-Setup-{version}.exe

#define AppName    "MGA Desktop"
#define AppPublisher "GreenFuze"
#define AppURL     "https://github.com/GreenFuze/MyGamesAnywhere"
; Version is read from the compiled assembly at build time.
; Override by passing /dAppVersion=x.y.z on the command line.
#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

#define AppExeName "MGA.Desktop.exe"
#define AppId      "{{B3E8C1A4-7F2D-4E5A-9B1C-3D6F8E2A0C47}"
; SourceRoot must be the publish output directory — set via /dSourceRoot=... on CLI
#ifndef SourceRoot
  #define SourceRoot "..\src\MGA.Desktop\bin\Release\net9.0\win-x64\publish"
#endif

[Setup]
AppId={#AppId}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/issues
DefaultDirName={localappdata}\Programs\{#AppName}
DefaultGroupName={#AppName}
AllowNoIcons=yes
PrivilegesRequired=lowest
OutputDir=Output
OutputBaseFilename=MGA-Setup-{#AppVersion}
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
CloseApplications=yes
CloseApplicationsFilter=*{#AppExeName}*
SetupIconFile=..\src\MGA.Desktop\Assets\mga.ico
UninstallDisplayIcon={app}\{#AppExeName}
VersionInfoVersion={#AppVersion}
VersionInfoCompany={#AppPublisher}
VersionInfoDescription={#AppName} Setup
; Minimum Windows 10 (build 18362) required for Acrylic support
MinVersion=10.0.18362

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
; Main application files
Source: "{#SourceRoot}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\{#AppName}";         Filename: "{app}\{#AppExeName}"
Name: "{group}\Uninstall {#AppName}"; Filename: "{uninstallexe}"
Name: "{commondesktop}\{#AppName}"; Filename: "{app}\{#AppExeName}"; Tasks: desktopicon

[Registry]
; Register mga:// URL scheme (user scope — no elevation needed)
Root: HKCU; Subkey: "Software\Classes\mga";                  ValueType: string; ValueName: "";                ValueData: "URL:MGA Protocol";     Flags: uninsdeletekey
Root: HKCU; Subkey: "Software\Classes\mga";                  ValueType: string; ValueName: "URL Protocol";   ValueData: ""
Root: HKCU; Subkey: "Software\Classes\mga\DefaultIcon";      ValueType: string; ValueName: "";                ValueData: "{app}\{#AppExeName},0"
Root: HKCU; Subkey: "Software\Classes\mga\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\{#AppExeName}"" ""%1"""

[Run]
Filename: "{app}\{#AppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(AppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; Remove local data created by the app (optional — prompts user).
; Commented out so user data is NOT deleted on uninstall by default.
; Type: filesandordirs; Name: "{localappdata}\MGA"

[Code]
// ---------------------------------------------------------------------------
// Bring existing window to front if already running
// ---------------------------------------------------------------------------
function InitializeSetup(): Boolean;
var
  hWnd: HWND;
begin
  Result := True;
end;
