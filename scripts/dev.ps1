<#
.SYNOPSIS
    在 Windows 上启动 Sinan 开发实例。

.DESCRIPTION
    封装现有的 `node scripts/desktop.mjs dev`：检查 Node.js/Go/Wails CLI，
    在前端依赖缺失时自动执行 setup，可选清理产物，并在构建前结束旧的开发实例，
    避免 bin\sinan.exe 被占用导致构建失败。全程使用 UTF-8 输出。

    默认行为：检查依赖 -> 结束旧开发实例 -> 构建 -> 准备 .sinan-dev 插件 -> 运行。

.PARAMETER Setup
    先执行 `node scripts/desktop.mjs setup`（go mod download + npm ci）。
    若缺少 wails3 CLI，同时执行 `go install ...@v3.0.0`。

.PARAMETER Clean
    删除 bin\ 与 .sinan-dev\ 后重新构建。脚本会校验目标位于仓库根目录内。

.PARAMETER NoStop
    不结束已运行的开发实例（若 bin\sinan.exe 正在运行，构建可能因文件占用失败）。

.PARAMETER ResetWebView
    启动前删除 .sinan-dev\webview2（仅开发实例的 WebView2 用户数据）。
    遇到 WebView2 0x800700aa“资源正在使用”或残留状态时可使用。

.PARAMETER DryRun
    只打印将要执行的步骤与依赖版本，不修改文件、不启动应用。

.EXAMPLE
    .\scripts\dev.cmd
    .\scripts\dev.cmd -Setup
    pwsh -File .\scripts\dev.ps1 -Clean -DryRun
#>
[CmdletBinding()]
param(
    [switch]$Setup,
    [switch]$Clean,
    [switch]$NoStop,
    [switch]$ResetWebView,
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# 与 AGENTS.md 一致：PowerShell 7 + UTF-8，避免中文乱码和 BOM 问题。
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)

$RequiredNode = [version]'22.12.0'
$WailsVersion = 'v3.0.0'
$Root = Split-Path -Parent $PSScriptRoot
$BinDir = Join-Path $Root 'bin'
$DevDir = Join-Path $Root '.sinan-dev'
$DevWebviewDir = Join-Path $DevDir 'webview2'
$FrontendModules = Join-Path $Root 'apps/desktop/frontend/node_modules'

function Write-Step([string]$Message) { Write-Host "[dev] $Message" -ForegroundColor Cyan }
function Write-Note([string]$Message) { Write-Host "      $Message" -ForegroundColor DarkGray }
function Fail([string]$Message) { Write-Host "[dev] $Message" -ForegroundColor Red; exit 1 }

function Invoke-Step([string]$Description, [scriptblock]$Action) {
    if ($DryRun) {
        Write-Step "（dry-run）$Description"
        return
    }
    Write-Step $Description
    & $Action
}

function Get-CommandPath([string]$Name) {
    $command = Get-Command $Name -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($command) { return $command.Source }
    return $null
}

function Test-InsideRoot([string]$Path) {
    $resolved = [System.IO.Path]::GetFullPath($Path)
    $prefix = [System.IO.Path]::GetFullPath($Root).TrimEnd('\') + '\'
    return $resolved.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)
}

Set-Location $Root

Write-Step "仓库：$Root"
if ($DryRun) { Write-Note 'dry-run：只检查环境并打印步骤，不会构建或启动应用。' }

# 1. 依赖检查
$node = Get-CommandPath 'node'
if (-not $node) { Fail '找不到 Node.js，请安装 Node.js 22.12+ 后重试。' }
$nodeVersionText = (& node -v).Trim()
if ($nodeVersionText -notmatch '^v(\d+\.\d+\.\d+)') { Fail "无法识别 Node.js 版本：$nodeVersionText" }
$nodeVersion = [version]$Matches[1]
if ($nodeVersion -lt $RequiredNode) { Fail "Node.js 版本过低（$nodeVersionText），需要 22.12+。" }
Write-Note "Node.js $nodeVersionText"

if (-not (Get-CommandPath 'npm.cmd') -and -not (Get-CommandPath 'npm')) { Fail '找不到 npm，请重新安装 Node.js。' }

$go = Get-CommandPath 'go'
if (-not $go) { Fail '找不到 Go，请安装 Go 1.26+ 后重试。' }
Write-Note (& go version)

# 让通过 go install 安装的 CLI 可被 node 脚本找到。
$goBin = ''
try { $goBin = (& go env GOPATH).Trim() } catch { $goBin = '' }
if ($goBin) {
    $goBinPath = Join-Path $goBin 'bin'
    if ((Test-Path $goBinPath) -and ($env:PATH -notlike "*$goBinPath*")) {
        $env:PATH = $goBinPath + [System.IO.Path]::PathSeparator + $env:PATH
    }
}

# 2. Wails CLI
$wails = Get-CommandPath 'wails3'
if (-not $wails -and $Setup -and -not $DryRun) {
    Write-Step "安装 Wails CLI（$WailsVersion）"
    & go install "github.com/wailsapp/wails/v3/cmd/wails3@$WailsVersion"
    if ($LASTEXITCODE -ne 0) { Fail '安装 Wails CLI 失败，请检查网络或 Go 代理设置。' }
    $wails = Get-CommandPath 'wails3'
}
if (-not $wails) {
    Fail "找不到 wails3 CLI。请运行：go install github.com/wailsapp/wails/v3/cmd/wails3@$WailsVersion，或加上 -Setup。"
}
Write-Note "Wails CLI：$wails"

# 3. 结束旧开发实例，避免 bin\sinan.exe 与 WebView2 数据目录被占用
function Stop-OldDevInstance {
    if ($NoStop) { Write-Note '按 -NoStop 跳过结束旧实例。'; return }
    if ($DryRun) { Write-Step '（dry-run）结束正在运行的开发实例（bin\sinan.exe / sinan-ui-host.exe）'; return }
    $stopped = $false
    foreach ($name in @('sinan', 'sinan-ui-host')) {
        foreach ($process in @(Get-Process -Name $name -ErrorAction SilentlyContinue)) {
            try {
                $path = $process.Path
            } catch {
                continue
            }
            if ($path -and (Test-InsideRoot $path) -and ($path -like "$BinDir\*")) {
                Write-Note "结束旧实例 $name (PID $($process.Id))"
                try { Stop-Process -Id $process.Id -Force -ErrorAction Stop; $stopped = $true } catch { Write-Note "无法结束 PID $($process.Id)：$($_.Exception.Message)" }
            }
        }
    }
    # 清理使用开发数据目录的残留 WebView2 子进程（主进程崩溃时可能遗留并锁住目录）。
    $orphans = @(Get-CimInstance Win32_Process -Filter "Name='msedgewebview2.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -and $_.CommandLine.Contains($DevWebviewDir) })
    foreach ($orphan in $orphans) {
        Write-Note "结束残留 WebView2 进程 (PID $($orphan.ProcessId))"
        try { Stop-Process -Id $orphan.ProcessId -Force -ErrorAction Stop; $stopped = $true } catch { }
    }
    if (-not $stopped) { return }
    # 等 WebView2 子进程释放用户数据目录，否则紧接着启动会报 0x800700aa。
    $deadline = (Get-Date).AddSeconds(8)
    while ((Get-Date) -lt $deadline) {
        $alive = @(Get-Process -Name 'sinan', 'sinan-ui-host' -ErrorAction SilentlyContinue)
        $webview = @(Get-CimInstance Win32_Process -Filter "Name='msedgewebview2.exe'" -ErrorAction SilentlyContinue |
            Where-Object { $_.CommandLine -and $_.CommandLine.Contains($DevWebviewDir) })
        if ($alive.Count -eq 0 -and $webview.Count -eq 0) { break }
        Start-Sleep -Milliseconds 250
    }
    if (@(Get-Process -Name 'sinan', 'sinan-ui-host' -ErrorAction SilentlyContinue).Count -gt 0) {
        Write-Note '旧实例仍在退出中；若稍后构建或启动失败，请手动结束后重试。'
    }
}
Invoke-Step '结束旧的开发实例（如正在运行）' { Stop-OldDevInstance }

# 4. 可选清理
if ($Clean) {
    foreach ($target in @($BinDir, $DevDir)) {
        if (-not (Test-Path $target)) { continue }
        if (-not (Test-InsideRoot $target)) { Fail "拒绝删除仓库外的路径：$target" }
        Invoke-Step "删除 $target" { Remove-Item -LiteralPath $target -Recurse -Force }
    }
}
if ($ResetWebView -and (Test-Path $DevWebviewDir)) {
    if (-not (Test-InsideRoot $DevWebviewDir)) { Fail "拒绝删除仓库外的路径：$DevWebviewDir" }
    Invoke-Step "重置开发 WebView2 数据（$DevWebviewDir）" { Remove-Item -LiteralPath $DevWebviewDir -Recurse -Force }
}

# 5. 依赖安装
$needsSetup = $Setup -or (-not (Test-Path $FrontendModules))
if ($needsSetup) {
    if (-not (Test-Path $FrontendModules)) { Write-Note '未找到前端依赖，将执行 setup。' }
    Invoke-Step '安装依赖（node scripts/desktop.mjs setup）' {
        & node scripts/desktop.mjs setup
        if ($LASTEXITCODE -ne 0) { Fail 'setup 失败。' }
    }
}

# 6. 启动开发实例
if ($DryRun) {
    Write-Note "开发 WebView2 数据：$DevWebviewDir"
    Write-Step '（dry-run）node scripts/desktop.mjs dev'
    Write-Step 'dry-run 结束，未构建、未修改文件、未启动桌面。'
    exit 0
}

Write-Host ''
Write-Step '启动开发实例：node scripts/desktop.mjs dev'
Write-Note '首次构建较慢；关闭窗口或 Ctrl+C 退出。'
& node scripts/desktop.mjs dev
if ($LASTEXITCODE -ne 0) {
    Write-Host "[dev] 开发实例以退出码 $LASTEXITCODE 结束。" -ForegroundColor Yellow
    Write-Note '若日志出现 WebView2 0x800700aa（资源正在使用），请退出其他司南/WebView2 进程后重试，或加上 -ResetWebView。'
    exit $LASTEXITCODE
}
