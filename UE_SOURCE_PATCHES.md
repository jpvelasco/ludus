# UE 5.6.1 Source Patches Required by Ludus

These patches are needed for UE 5.6.1 when building on Windows with SDK >= 26100 (Windows 11 24H2+).
When Epic fixes these in a future UE release, the corresponding patches can be removed.

## 1. NNERuntimeORT: Missing INITGUID for DXCore GUID (Windows SDK >= 26100)

**File**: `Engine/Plugins/NNE/NNERuntimeORT/Source/NNERuntimeORT/NNERuntimeORT.Build.cs`

**Problem**: When Windows SDK >= 26100, the module defines `ORT_USE_NEW_DXCORE_FEATURES` which
enables code that references `DXCORE_ADAPTER_ATTRIBUTE_D3D12_GENERIC_ML` from `dxcore_interface.h`.
This GUID is declared with `DEFINE_GUID` which, without `INITGUID` defined, produces only an
`extern` declaration. The linker then fails with:
```
error LNK2001: unresolved external symbol DXCORE_ADAPTER_ATTRIBUTE_D3D12_GENERIC_ML
```
This affects both `UnrealEditor-NNERuntimeORT.dll` and `LyraGame.exe`.

**Fix**: Add `PublicDefinitions.Add("INITGUID")` inside the `WindowsSdkVersion.Build >= 26100` block:
```csharp
if (Version.TryParse(Target.WindowsPlatform.WindowsSdkVersion, out Version WindowsSdkVersion) && WindowsSdkVersion.Build >= 26100)
{
    PublicDefinitions.Add("ORT_USE_NEW_DXCORE_FEATURES");
    PublicDefinitions.Add("INITGUID");  // <-- added
}
```

**Status**: Bug in UE 5.6.1. Expected to be fixed in 5.6.2+.

---

## 2. MSVC Toolchain Version (VS 2025/2026 Compatibility)

**File**: `%APPDATA%\Unreal Engine\UnrealBuildTool\BuildConfiguration.xml` (user config, not source)

**Problem**: UE 5.6.1 expects MSVC 14.38 (VS 2022 v143 toolset). If the user has VS 2025/2026
(MSVC 14.50+), the newer compiler triggers warnings promoted to errors:
- `C4756`: overflow in constant arithmetic (AnimNextAnimGraph `ITimeline.h`)
- `C4458`: declaration hides class member (RigLogicLib `FilteredBinaryInputArchive.cpp`)

**Fix**: Install the MSVC 14.38 toolchain via VS Installer:
```
Microsoft.VisualStudio.Component.VC.14.38.17.8.x86.x64
```
Then configure UBT to use it:
```xml
<?xml version="1.0" encoding="utf-8" ?>
<Configuration xmlns="https://www.unrealengine.com/BuildConfiguration">
  <WindowsPlatform>
    <CompilerVersion>14.38.33130</CompilerVersion>
  </WindowsPlatform>
</Configuration>
```

**Status**: Not a bug per se — VS 2025/2026 isn't officially supported by UE 5.6.1 yet.
Ludus should auto-detect and handle this in `ludus init`.

---

## 3. Runtime Patches Applied by Ludus Code (Not Source Edits)

These are applied automatically by ludus at build time:

### NuGet Audit Level (`ensureNuGetAuditDisabled`)
**File**: `Engine/Source/Programs/Directory.Build.props` (created at runtime)
**Reason**: UE 5.6's Gauntlet test framework depends on Magick.NET 14.7.0 with known CVEs.
Combined with `TreatWarningsAsErrors`, AutomationTool script modules fail to compile.
Setting `NuGetAuditLevel=critical` allows non-critical CVEs through.

### Default Server Target (`ensureDefaultServerTarget`)
**File**: `Samples/Games/Lyra/Config/DefaultEngine.ini` (modified at runtime)
**Reason**: UE 5.6 Lyra ships with multiple server targets. `DefaultServerTarget=LyraServer`
must be set for RunUAT to know which target to build.

---

## 4. Lyra Content Copy Must Include Plugin Content (Setup Issue, Not Source Patch)

**Problem**: The UE5 GitHub source repository includes Lyra's GameFeature plugin structure
(C++ code, configs, `.uplugin` files) but NOT the Content directories. These are part of the
"Lyra Starter Game" download from the Epic Games Launcher Marketplace.

When copying Lyra content into the engine source tree, it's critical to copy the **entire
downloaded project** over `Samples/Games/Lyra/`, not just the top-level `Content/` directory.
Each GameFeature plugin has its own `Content/` folder:

```
Plugins/GameFeatures/ShooterCore/Content/     (260 files, includes ShooterCore.uasset)
Plugins/GameFeatures/ShooterExplorer/Content/  (51 files, includes ShooterExplorer.uasset)
Plugins/GameFeatures/ShooterMaps/Content/      (5330 files, includes ShooterMaps.uasset)
Plugins/GameFeatures/ShooterTests/Content/     (35 files, includes ShooterTests.uasset)
Plugins/GameFeatures/TopDownArena/Content/     (87 files, includes TopDownArena.uasset)
```

Missing these causes the cook phase to fail with `ExitCode=25 (Error_UnknownCookFailure)`:
```
LogGameFeatures: Error: Setting file:.../TopDownArena.uplugin to be in unrecoverable error as GameFeatureData is missing
LogGameFeatures: Error: Setting file:.../ShooterCore.uplugin to be in unrecoverable error as GameFeatureData is missing
```

**Fix**: Ludus should copy the entire downloaded Lyra project folder over the engine's
`Samples/Games/Lyra/` directory (overlay, not replace — preserving existing source files).
The `ludus init` or prereq check should verify that `Plugins/GameFeatures/*/Content/` dirs exist.

**Status**: This is by design (Epic splits source and content). Ludus needs to handle this
in the content copy step, ensuring plugin content is included.

---

## Validation Checklist for Future UE Versions

When upgrading UE, check if these are still needed:
- [ ] NNERuntimeORT links correctly without INITGUID on SDK >= 26100
- [ ] AnimNextAnimGraph compiles without C4756 on the latest MSVC
- [ ] RigLogicLib compiles without C4458 on the latest MSVC
- [ ] Gauntlet's Magick.NET dependency is updated (no more NuGet audit failures)
- [ ] Lyra's DefaultEngine.ini includes DefaultServerTarget by default
- [ ] Lyra Marketplace download includes GameFeature plugin Content in a single package (no separate copy needed)
