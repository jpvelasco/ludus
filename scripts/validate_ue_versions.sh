#!/usr/bin/env bash
# validate_ue_versions.sh — Structural validation of UE source releases against
# Ludus's assumptions. Extracts only the specific files Ludus touches and checks
# compatibility without building anything.
set -euo pipefail

DOWNLOADS_DIR="${1:-$HOME/Downloads}"
WORK_DIR=$(mktemp -d)
REPORT_FILE="$DOWNLOADS_DIR/ue_version_compatibility_report.txt"

trap 'rm -rf "$WORK_DIR"' EXIT

TARBALLS=(
    "UnrealEngine-5.4.4-release.tar.gz"
    "UnrealEngine-5.5.4-release.tar.gz"
    "UnrealEngine-5.6.1-release.tar.gz"
    "UnrealEngine-5.7.3-release.tar.gz"
)

PASS="[PASS]"
FAIL="[FAIL]"
WARN="[WARN]"
INFO="[INFO]"

{
echo "========================================================================"
echo "  Ludus — UE Version Compatibility Report"
echo "  Generated: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo "========================================================================"
echo ""

for tarball in "${TARBALLS[@]}"; do
    tarpath="$DOWNLOADS_DIR/$tarball"
    version="${tarball#UnrealEngine-}"
    version="${version%-release.tar.gz}"

    echo "------------------------------------------------------------------------"
    echo "  UE $version"
    echo "------------------------------------------------------------------------"
    echo ""

    if [[ ! -f "$tarpath" ]]; then
        echo "  $FAIL Tarball not found: $tarpath"
        echo ""
        continue
    fi

    echo "  $INFO Tarball: $tarball ($(du -h "$tarpath" | cut -f1))"

    # Step 1: Generate full file listing ONCE (takes ~30-60s per tarball)
    echo "  $INFO Generating file listing..."
    LISTING="$WORK_DIR/${version}_listing.txt"
    tar -tzf "$tarpath" > "$LISTING"
    FILE_COUNT=$(wc -l < "$LISTING")
    echo "  $INFO $FILE_COUNT files in archive"

    # Discover the top-level directory prefix
    PREFIX=$(head -1 "$LISTING")
    PREFIX="${PREFIX%%/*}"
    echo "  $INFO Archive prefix: $PREFIX"
    echo ""

    # Step 2: Extract only the files we need to inspect
    VDIR="$WORK_DIR/$version"
    mkdir -p "$VDIR"

    EXTRACT_FILES=(
        "$PREFIX/Engine/Build/Build.version"
        "$PREFIX/Engine/Plugins/NNE/NNERuntimeORT/Source/NNERuntimeORT/NNERuntimeORT.Build.cs"
        "$PREFIX/Samples/Games/Lyra/Config/DefaultEngine.ini"
        "$PREFIX/Engine/Source/Programs/Directory.Build.props"
    )
    for f in "${EXTRACT_FILES[@]}"; do
        tar -xzf "$tarpath" -C "$VDIR" "$f" 2>/dev/null || true
    done

    EDIR="$VDIR/$PREFIX"

    # ---------------------------------------------------------------
    # 1. Engine/Build/Build.version
    # ---------------------------------------------------------------
    echo "  --- Build.version ---"
    BV_PATH="$EDIR/Engine/Build/Build.version"
    ENGINE_VER=""
    if [[ -f "$BV_PATH" ]]; then
        MAJOR=$(python3 -c "import json; d=json.load(open('$BV_PATH')); print(d['MajorVersion'])" 2>/dev/null || echo "?")
        MINOR=$(python3 -c "import json; d=json.load(open('$BV_PATH')); print(d['MinorVersion'])" 2>/dev/null || echo "?")
        PATCH=$(python3 -c "import json; d=json.load(open('$BV_PATH')); print(d['PatchVersion'])" 2>/dev/null || echo "?")
        ENGINE_VER="${MAJOR}.${MINOR}"
        echo "  $PASS Build.version found: ${MAJOR}.${MINOR}.${PATCH}"

        case "$ENGINE_VER" in
            5.4|5.5|5.6|5.7)
                echo "  $PASS Engine $ENGINE_VER is in Ludus toolchainMap"
                ;;
            *)
                echo "  $FAIL Engine $ENGINE_VER is NOT in Ludus toolchainMap (needs entry)"
                ;;
        esac
    else
        echo "  $FAIL Build.version not found at expected path"
    fi
    echo ""

    # ---------------------------------------------------------------
    # 2. NNERuntimeORT.Build.cs (INITGUID patch target)
    # ---------------------------------------------------------------
    echo "  --- NNERuntimeORT.Build.cs (INITGUID patch) ---"
    ORT_PATH="$EDIR/Engine/Plugins/NNE/NNERuntimeORT/Source/NNERuntimeORT/NNERuntimeORT.Build.cs"
    if [[ -f "$ORT_PATH" ]]; then
        echo "  $PASS File exists at expected path"

        if grep -q "INITGUID" "$ORT_PATH"; then
            echo "  $PASS INITGUID already present — patch NOT needed"
        else
            echo "  $WARN INITGUID not present — patch may be needed (Windows SDK >= 26100)"
        fi

        if grep -q "ORT_USE_NEW_DXCORE_FEATURES" "$ORT_PATH"; then
            echo "  $WARN Contains ORT_USE_NEW_DXCORE_FEATURES — DXCore GUID bug may apply"
        else
            echo "  $INFO No ORT_USE_NEW_DXCORE_FEATURES reference — DXCore issue N/A"
        fi

        if grep -q "PublicDependencyModuleNames" "$ORT_PATH"; then
            echo "  $PASS PublicDependencyModuleNames marker found (Ludus auto-fix insertion point OK)"
        else
            echo "  $FAIL PublicDependencyModuleNames marker NOT found — Ludus auto-fix would FAIL"
        fi
    else
        if grep -q "Engine/Plugins/NNE/" "$LISTING"; then
            echo "  $WARN NNE plugin exists but NNERuntimeORT.Build.cs at different path"
            grep "NNERuntimeORT.Build.cs" "$LISTING" | head -3 | while read -r p; do
                echo "  $INFO   Found: $p"
            done
        else
            echo "  $INFO NNE plugin does not exist in this version — INITGUID patch N/A"
        fi
    fi
    echo ""

    # ---------------------------------------------------------------
    # 3. Lyra project structure
    # ---------------------------------------------------------------
    echo "  --- Lyra project ---"
    LYRA_INI="$EDIR/Samples/Games/Lyra/Config/DefaultEngine.ini"

    if grep -q "$PREFIX/Samples/Games/Lyra/Lyra.uproject" "$LISTING"; then
        echo "  $PASS Lyra.uproject found at expected path"
    else
        LYRA_HIT=$(grep "Lyra.uproject" "$LISTING" | head -1 || echo "")
        if [[ -n "$LYRA_HIT" ]]; then
            echo "  $WARN Lyra.uproject found at different path: $LYRA_HIT"
        else
            echo "  $FAIL Lyra.uproject not found anywhere in archive"
        fi
    fi

    if [[ -f "$LYRA_INI" ]]; then
        echo "  $PASS DefaultEngine.ini found"

        if grep -q "DefaultServerTarget" "$LYRA_INI"; then
            echo "  $PASS DefaultServerTarget already set — Ludus patch NOT needed"
        else
            echo "  $WARN DefaultServerTarget not set — Ludus will add it at build time"
        fi

        if grep -q "DefaultGameTarget=" "$LYRA_INI"; then
            GAME_TARGET=$(grep "DefaultGameTarget=" "$LYRA_INI" | head -1 | tr -d '\r')
            echo "  $PASS Found: $GAME_TARGET (Ludus insertion anchor present)"
        else
            echo "  $WARN DefaultGameTarget= not found — Ludus DefaultServerTarget patch would SKIP"
        fi
    else
        echo "  $INFO DefaultEngine.ini not extracted (may not exist in source — Lyra Content needed)"
    fi

    echo ""
    echo "  --- Lyra GameFeature plugins (source structure) ---"
    PLUGINS=("ShooterCore" "ShooterMaps" "TopDownArena" "ShooterExplorer" "ShooterTests")
    for plugin in "${PLUGINS[@]}"; do
        if grep -q "$PREFIX/Samples/Games/Lyra/Plugins/GameFeatures/$plugin/" "$LISTING"; then
            echo "  $PASS $plugin plugin directory exists"
        else
            echo "  $WARN $plugin plugin directory NOT found"
        fi
    done
    echo ""

    # ---------------------------------------------------------------
    # 4. Linux cross-compile toolchain (bundled SDK)
    # ---------------------------------------------------------------
    echo "  --- Linux cross-compile toolchain ---"
    TC_DIRS=$(grep "$PREFIX/Engine/Extras/ThirdPartyNotUE/SDKs/HostLinux/Linux_x64/" "$LISTING" 2>/dev/null \
        | grep -oP "Linux_x64/[^/]+" | sort -u || echo "")
    if [[ -n "$TC_DIRS" ]]; then
        echo "  $PASS SDK directory exists. Bundled toolchains:"
        while read -r tc; do
            tc_name="${tc#Linux_x64/}"
            echo "  $INFO   $tc_name"
        done <<< "$TC_DIRS"
    else
        echo "  $WARN HostLinux/Linux_x64 SDK path not found (toolchain needs manual install via Setup.sh)"
    fi
    echo ""

    # ---------------------------------------------------------------
    # 5. Directory.Build.props (NuGet audit)
    # ---------------------------------------------------------------
    echo "  --- Directory.Build.props (NuGet audit) ---"
    DBPROPS="$EDIR/Engine/Source/Programs/Directory.Build.props"
    if [[ -f "$DBPROPS" ]]; then
        echo "  $INFO Directory.Build.props exists in source tree"
        if grep -q "NuGetAudit" "$DBPROPS"; then
            echo "  $INFO Contains NuGet audit config"
        fi
    else
        echo "  $INFO Not present (Ludus uses env var workaround — no file modification needed)"
    fi
    echo ""

    # ---------------------------------------------------------------
    # 6. Setup scripts
    # ---------------------------------------------------------------
    echo "  --- Setup scripts ---"
    if grep -q "^$PREFIX/Setup.sh$" "$LISTING"; then
        echo "  $PASS Setup.sh found (Linux engine source check passes)"
    else
        echo "  $FAIL Setup.sh NOT found (Ludus engine source validation would FAIL)"
    fi
    if grep -q "^$PREFIX/Setup.bat$" "$LISTING"; then
        echo "  $PASS Setup.bat found (Windows engine source check passes)"
    else
        echo "  $FAIL Setup.bat NOT found (Ludus Windows validation would FAIL)"
    fi
    echo ""

    # ---------------------------------------------------------------
    # Summary
    # ---------------------------------------------------------------
    echo "  --- Ludus version-gated patch summary for UE $version ---"
    if [[ "$ENGINE_VER" == "5.6" ]]; then
        echo "  $INFO INITGUID patch:          APPLIES (gated to 5.6)"
        echo "  $INFO NuGet audit workaround:   APPLIES (gated to 5.6)"
    else
        echo "  $INFO INITGUID patch:          SKIPPED (gated to 5.6 only)"
        echo "  $INFO NuGet audit workaround:   SKIPPED (gated to 5.6 only)"
    fi
    echo "  $INFO DefaultServerTarget:     APPLIES (no version gate — graceful skip if anchor missing)"
    echo "  $WARN MSVC 14.38 pin:          APPLIES UNCONDITIONALLY — may be wrong for $ENGINE_VER"
    echo ""
    echo ""

    # Clean up extracted files
    rm -rf "$VDIR"
done

echo "========================================================================"
echo "  End of report"
echo "========================================================================"

} 2>&1 | tee "$REPORT_FILE"

echo ""
echo "Report saved to: $REPORT_FILE"
