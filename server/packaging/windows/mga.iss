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
#ifdef UNICODE
#define AW "W"
#else
#define AW "A"
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
PrivilegesRequiredOverridesAllowed=commandline
UsePreviousPrivileges=no
UninstallDisplayIcon={app}\{#AppExeName}
CloseApplications=yes
RestartApplications=no
SetupLogging=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "startup_server"; Description: "Start MGA automatically when I sign in"; Flags: checkedonce; Check: IsForMeMode
Name: "start_server_after"; Description: "Start MGA after install"; Flags: checkedonce; Check: IsForMeMode
Name: "startup_tray"; Description: "Show MGA tray icon when I sign in"; Flags: dontinheritcheck; Check: IsAllUsersMode
Name: "start_service_after"; Description: "Start MGA service after install"; Flags: dontinheritcheck; Check: IsAllUsersMode
Name: "firewall"; Description: "Add Windows Firewall rule for LAN access"; Flags: checkedonce; Check: IsAllUsersMode

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
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "MyGamesAnywhere"; ValueData: """{app}\{#AppExeName}"" --app-dir ""{app}"" --data-dir ""{code:GetDataDir}"" --config ""{code:GetConfigPath}"" --runtime-mode ""{code:GetRuntimeMode}"""; Tasks: startup_server; Flags: uninsdeletevalue
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "MyGamesAnywhere Tray"; ValueData: """{app}\{#TrayExeName}"" --base-url ""http://127.0.0.1:8900"" --mode ""service"" --server-exe ""{app}\{#AppExeName}"" --app-dir ""{app}"" --data-dir ""{code:GetDataDir}"" --config ""{code:GetConfigPath}"" --runtime-mode ""{code:GetRuntimeMode}"""; Tasks: startup_tray; Flags: uninsdeletevalue

[Run]
Filename: "{app}\{#AppExeName}"; Parameters: "--app-dir ""{app}"" --data-dir ""{code:GetDataDir}"" --config ""{code:GetConfigPath}"" --runtime-mode ""{code:GetRuntimeMode}"""; Tasks: start_server_after; Flags: nowait postinstall skipifsilent; Check: IsForMeMode

[UninstallRun]
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\service.ps1"" -Action uninstall"; Flags: runhidden waituntilterminated; RunOnceId: "RemoveMGAService"
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\firewall.ps1"" -Action remove"; Flags: runhidden waituntilterminated; RunOnceId: "RemoveMGAFirewall"

[Code]
var
  InstallScopePage: TInputOptionWizardPage;

procedure ExitProcess(ExitCode: UINT);
  external 'ExitProcess@kernel32.dll stdcall';

function ShellExecute(hwnd: HWND; Operation: String; FileName: String; Parameters: String; Directory: String; ShowCmd: Integer): THandle;
  external 'ShellExecute{#AW}@shell32.dll stdcall';

function DQ(Value: String): String;
begin
  Result := '"' + Value + '"';
end;

function PSArg(Name: String; Value: String): String;
begin
  Result := '-' + Name + ' ' + DQ(Value);
end;

function ReadFailureDetails(LogPath: String): String;
begin
  Result := '';
  if LogPath = '' then
    Exit;

  if not FileExists(LogPath) then
  begin
    Result := 'No MGA install log was found at: ' + LogPath;
    Exit;
  end;

  Result := 'Details were written to: ' + LogPath;
end;

procedure RunPowerShellStep(ScriptName: String; Parameters: String; Description: String; DetailsPath: String);
var
  ResultCode: Integer;
  PowerShellParameters: String;
  Details: String;
begin
  PowerShellParameters := '-NoProfile -ExecutionPolicy Bypass -File ' + DQ(ExpandConstant('{app}\') + ScriptName);
  if Parameters <> '' then
    PowerShellParameters := PowerShellParameters + ' ' + Parameters;

  Log('Running ' + Description + ': powershell.exe ' + PowerShellParameters);
  if not Exec('powershell.exe', PowerShellParameters, '', SW_HIDE, ewWaitUntilTerminated, ResultCode) then
  begin
    Log(Description + ' failed to start. Error code: ' + IntToStr(ResultCode));
    RaiseException(Description + ' failed to start: ' + SysErrorMessage(ResultCode));
  end;

  if ResultCode <> 0 then
  begin
    Log(Description + ' failed. Exit code: ' + IntToStr(ResultCode));
    Details := ReadFailureDetails(DetailsPath);
    if Details <> '' then
      RaiseException(Description + ' failed. Exit code: ' + IntToStr(ResultCode) + #13#10#13#10 + Details)
    else
      RaiseException(Description + ' failed. Exit code: ' + IntToStr(ResultCode));
  end;
end;

function IsAllUsersMode: Boolean;
begin
  if InstallScopePage <> nil then
    Result := InstallScopePage.SelectedValueIndex = 1
  else
    Result := IsAdminInstallMode;
end;

function IsForMeMode: Boolean;
begin
  Result := not IsAllUsersMode;
end;

function IsServiceInstall: Boolean;
begin
  Result := IsAllUsersMode;
end;

function GetDefaultDir(Param: String): String;
begin
  if IsAllUsersMode then
    Result := ExpandConstant('{autopf}\MyGamesAnywhere')
  else
    Result := ExpandConstant('{localappdata}\Programs\MyGamesAnywhere');
end;

function GetDataDir(Param: String): String;
begin
  if IsAllUsersMode then
    Result := ExpandConstant('{commonappdata}\MyGamesAnywhere')
  else
    Result := ExpandConstant('{localappdata}\MyGamesAnywhere');
end;

function GetConfigPath(Param: String): String;
begin
  Result := GetDataDir('') + '\config.json';
end;

function GetInstallLogPath(Param: String): String;
begin
  Result := GetDataDir('') + '\mga_install.log';
end;

function GetRuntimeMode(Param: String): String;
begin
  if IsAllUsersMode then
    Result := 'machine'
  else
    Result := 'user';
end;

function GetInstallType(Param: String): String;
begin
  if IsAllUsersMode then
    Result := 'service'
  else
    Result := 'user';
end;

function GetTrayMode(Param: String): String;
begin
  if IsAllUsersMode then
    Result := 'service'
  else
    Result := 'process';
end;

function GetListenMode(Param: String): String;
begin
  if IsAllUsersMode then
    Result := 'lan'
  else
    Result := 'local';
end;

function ShouldAddFirewall: Boolean;
begin
  Result := IsAllUsersMode and WizardIsTaskSelected('firewall');
end;

procedure InitializeWizard;
begin
  InstallScopePage := CreateInputOptionPage(
    wpWelcome,
    'Choose how MGA should run',
    'Pick the installation mode for this machine.',
    'For me only installs MGA under your Windows profile. It runs when you sign in and only this PC can access MGA by default.' + #13#10#13#10 +
    'All users installs MGA as a Windows service. It requires administrator approval, starts before login, and other LAN devices can access MGA by default.',
    True,
    False);
  InstallScopePage.Add('For me only');
  if IsAdminInstallMode then
  begin
    InstallScopePage.Add('All users');
    InstallScopePage.SelectedValueIndex := 1
  end
  else
  begin
    InstallScopePage.Add('All users (restarts installer)');
    InstallScopePage.SelectedValueIndex := 0;
  end;
end;

function RelaunchAllUsers: Boolean;
var
  RetVal: THandle;
  Params: String;
begin
  Params := '/ALLUSERS /LANG=' + ActiveLanguage;
  RetVal := ShellExecute(WizardForm.Handle, 'runas', ExpandConstant('{srcexe}'), Params, '', SW_SHOW);
  Result := RetVal > 32;
  if Result then
    ExitProcess(0)
  else
    MsgBox('Administrator approval is required for All users mode. Windows returned: ' + SysErrorMessage(RetVal), mbError, MB_OK);
end;

procedure ApplyScopeDefaults;
begin
  if IsAllUsersMode then
    WizardSelectTasks('startup_tray,start_service_after,firewall')
  else
    WizardSelectTasks('startup_server,start_server_after');
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;

  if (CurPageID = InstallScopePage.ID) then
  begin
    if (InstallScopePage.SelectedValueIndex = 1) and not IsAdminInstallMode then
    begin
      RelaunchAllUsers;
      Result := False;
      Exit;
    end;
    ApplyScopeDefaults;
  end;

end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  ConfigParameters: String;
  ServiceParameters: String;
  FirewallParameters: String;
begin
  if CurStep = ssPostInstall then
  begin
    ConfigParameters :=
      PSArg('AppDir', ExpandConstant('{app}')) + ' ' +
      PSArg('DataDir', GetDataDir('')) + ' ' +
      PSArg('ListenMode', GetListenMode('')) + ' ' +
      PSArg('InstallType', GetInstallType(''));
    RunPowerShellStep('install-config.ps1', ConfigParameters, 'MGA config generation', '');

    if IsServiceInstall then
    begin
      ServiceParameters :=
        '-Action install ' +
        PSArg('AppDir', ExpandConstant('{app}')) + ' ' +
        PSArg('DataDir', GetDataDir('')) + ' ' +
        PSArg('ConfigPath', GetConfigPath('')) + ' ' +
        PSArg('LogPath', GetInstallLogPath(''));
      RunPowerShellStep('service.ps1', ServiceParameters, 'MGA service installation', GetInstallLogPath(''));

      if ShouldAddFirewall then
      begin
        FirewallParameters :=
          '-Action add ' +
          PSArg('Program', ExpandConstant('{app}\{#AppExeName}'));
        RunPowerShellStep('firewall.ps1', FirewallParameters, 'MGA firewall rule installation', '');
      end;

      if WizardIsTaskSelected('start_service_after') then
        RunPowerShellStep(
          'service.ps1',
          '-Action start ' +
          PSArg('AppDir', ExpandConstant('{app}')) + ' ' +
          PSArg('DataDir', GetDataDir('')) + ' ' +
          PSArg('ConfigPath', GetConfigPath('')) + ' ' +
          PSArg('LogPath', GetInstallLogPath('')),
          'MGA service start',
          GetInstallLogPath(''));
    end;
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
