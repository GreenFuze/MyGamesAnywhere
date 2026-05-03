#define AppName "MyGamesAnywhere"
#define AppPublisher "GreenFuze"
#define AppExeName "mga_server.exe"
#define TrayExeName "mga_tray.exe"
#ifndef MyAppVersion
#define MyAppVersion "0.0.0"
#endif
#ifndef SourceDir
#define SourceDir "..\..\bin"
#endif
#ifndef OutputDir
#define OutputDir "..\..\release"
#endif

[Setup]
AppId={{9B4F5E66-70B4-4F9-B9C0-D01A00000001}
AppName={#AppName}
AppVersion={#MyAppVersion}
AppPublisher={#AppPublisher}
DefaultDirName={code:GetDefaultDir}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
OutputDir={#OutputDir}
OutputBaseFilename=mga-v{#MyAppVersion}-windows-amd64-installer
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
UninstallDisplayIcon={app}\{#AppExeName}
CloseApplications=yes
RestartApplications=no

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "runtime_process"; Description: "Run MGA as a user process"; GroupDescription: "Runtime mode:"; Flags: exclusive checkedonce
Name: "runtime_service"; Description: "Run MGA as a Windows service"; GroupDescription: "Runtime mode:"; Flags: exclusive unchecked
Name: "startup_tray"; Description: "Start tray companion when I sign in"; Flags: checkedonce
Name: "start_after"; Description: "Start MGA after install"; Flags: checkedonce
Name: "listen_lan"; Description: "Allow devices on my LAN to connect"; Flags: unchecked
Name: "firewall"; Description: "Add Windows Firewall rule for LAN access"; Flags: unchecked

[Files]
Source: "{#SourceDir}\{#AppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\{#TrayExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\mga.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\config.json"; DestDir: "{app}"; DestName: "config.portable.json"; Flags: ignoreversion skipifsourcedoesntexist
Source: "{#SourceDir}\frontend\*"; DestDir: "{app}\frontend"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#SourceDir}\plugins\*"; DestDir: "{app}\plugins"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#SourceDir}\LICENSE.md"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist
Source: "{#SourceDir}\NOTICE"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist
Source: "{#SourceDir}\README.md"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist
Source: "{#SourceDir}\packaging\windows\install-config.ps1"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\packaging\windows\service.ps1"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\packaging\windows\firewall.ps1"; DestDir: "{app}"; Flags: ignoreversion

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "MyGamesAnywhere Tray"; ValueData: """{app}\{#TrayExeName}"" --base-url ""http://127.0.0.1:8900"" --mode ""{code:GetTrayMode}"" --server-exe ""{app}\{#AppExeName}"" --app-dir ""{app}"" --data-dir ""{code:GetDataDir}"" --config ""{code:GetConfigPath}"" --runtime-mode ""{code:GetRuntimeMode}"""; Tasks: startup_tray; Flags: uninsdeletevalue

[Run]
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\install-config.ps1"" -AppDir ""{app}"" -DataDir ""{code:GetDataDir}"" -ListenMode ""{code:GetListenMode}"" -InstallType ""{code:GetInstallType}"""; Flags: runhidden waituntilterminated
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\service.ps1"" -Action install -AppDir ""{app}"" -DataDir ""{code:GetDataDir}"" -ConfigPath ""{code:GetConfigPath}"""; Tasks: runtime_service; Flags: runhidden waituntilterminated; Check: IsAdminInstallMode
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\firewall.ps1"" -Action add -Program ""{app}\{#AppExeName}"""; Tasks: firewall; Flags: runhidden waituntilterminated; Check: ShouldAddFirewall
Filename: "{app}\{#TrayExeName}"; Parameters: "--base-url ""http://127.0.0.1:8900"" --mode ""{code:GetTrayMode}"" --server-exe ""{app}\{#AppExeName}"" --app-dir ""{app}"" --data-dir ""{code:GetDataDir}"" --config ""{code:GetConfigPath}"" --runtime-mode ""{code:GetRuntimeMode}"""; Tasks: start_after; Flags: nowait postinstall skipifsilent
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\service.ps1"" -Action start"; Tasks: runtime_service start_after; Flags: runhidden waituntilterminated; Check: IsAdminInstallMode

[UninstallRun]
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\service.ps1"" -Action uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "RemoveMGAService"
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\firewall.ps1"" -Action remove"; Flags: runhidden waituntilterminated; RunOnceId: "RemoveMGAFirewall"

[Code]
function GetDefaultDir(Param: String): String;
begin
  if IsAdminInstallMode then
    Result := ExpandConstant('{autopf}\MyGamesAnywhere')
  else
    Result := ExpandConstant('{localappdata}\Programs\MyGamesAnywhere');
end;

function GetDataDir(Param: String): String;
begin
  if IsAdminInstallMode or WizardIsTaskSelected('runtime_service') then
    Result := ExpandConstant('{commonappdata}\MyGamesAnywhere')
  else
    Result := ExpandConstant('{localappdata}\MyGamesAnywhere');
end;

function GetConfigPath(Param: String): String;
begin
  Result := GetDataDir('') + '\config.json';
end;

function GetRuntimeMode(Param: String): String;
begin
  if IsAdminInstallMode or WizardIsTaskSelected('runtime_service') then
    Result := 'machine'
  else
    Result := 'user';
end;

function GetInstallType(Param: String): String;
begin
  if WizardIsTaskSelected('runtime_service') then
    Result := 'service'
  else if IsAdminInstallMode then
    Result := 'machine'
  else
    Result := 'user';
end;

function GetTrayMode(Param: String): String;
begin
  if WizardIsTaskSelected('runtime_service') then
    Result := 'service'
  else
    Result := 'process';
end;

function GetListenMode(Param: String): String;
begin
  if WizardIsTaskSelected('listen_lan') then
    Result := 'lan'
  else
    Result := 'local';
end;

function ShouldAddFirewall: Boolean;
begin
  Result := IsAdminInstallMode and WizardIsTaskSelected('listen_lan') and WizardIsTaskSelected('firewall');
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if (CurPageID = wpSelectTasks) and WizardIsTaskSelected('runtime_service') and not IsAdminInstallMode then
  begin
    MsgBox('Windows service mode requires an all-users/admin install.', mbError, MB_OK);
    Result := False;
  end;
  if (CurPageID = wpSelectTasks) and WizardIsTaskSelected('firewall') and not IsAdminInstallMode then
  begin
    MsgBox('Adding the Windows Firewall rule requires an all-users/admin install.', mbError, MB_OK);
    Result := False;
  end;
  if (CurPageID = wpSelectTasks) and WizardIsTaskSelected('firewall') and not WizardIsTaskSelected('listen_lan') then
  begin
    MsgBox('The Windows Firewall rule is only available when LAN access is enabled.', mbError, MB_OK);
    Result := False;
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  DataDir: String;
begin
  if CurUninstallStep = usPostUninstall then
  begin
    if IsAdminInstallMode then
      DataDir := ExpandConstant('{commonappdata}\MyGamesAnywhere')
    else
      DataDir := ExpandConstant('{localappdata}\MyGamesAnywhere');

    if DirExists(DataDir) then
    begin
      if MsgBox('Delete MGA user data at ' + DataDir + '? Choose No to keep your library, settings, media, and update cache.', mbConfirmation, MB_YESNO) = IDYES then
        DelTree(DataDir, True, True, True);
    end;
  end;
end;
