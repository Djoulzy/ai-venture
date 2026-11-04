#!/usr/bin/env bash
set -euo pipefail

# Pinned upstream release: first stable release with LFM3 support.
llama_repository=https://github.com/ggml-org/llama.cpp.git
llama_release=v0.8.0
llama_commit=8642541b255bfb5dc39cd351a178be09013a55d6
cmake_version=3.31.6

project_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source_dir="$project_dir/third_party/llama.cpp"
build_dir="$project_dir/build/llama.cpp"
tools_dir="$project_dir/.tools/llama-build"
output_dir="$project_dir/runtime"
build_jobs=${LLAMA_BUILD_JOBS:-4}

if [[ ! "$build_jobs" =~ ^[1-9][0-9]*$ ]]; then
    echo "LLAMA_BUILD_JOBS doit être un entier positif." >&2
    exit 1
fi
case "$(uname -s)" in
    Darwin) metal=ON ;;
    Linux) metal=OFF ;;
    *) echo "Ce script de compilation prend en charge macOS et Linux." >&2; exit 1 ;;
esac
for tool in git python3 make; do
    command -v "$tool" >/dev/null || { echo "Outil requis : $tool" >&2; exit 1; }
done

mkdir -p "$project_dir/third_party" "$output_dir"
if [[ ! -e "$source_dir" ]]; then
    git clone --depth 1 --branch "$llama_release" "$llama_repository" "$source_dir"
fi
actual_commit=$(git -C "$source_dir" rev-parse HEAD)
if [[ "$actual_commit" != "$llama_commit" ]]; then
    echo "Révision llama.cpp inattendue : $actual_commit (attendu : $llama_commit)." >&2
    echo "Les sources existantes sont conservées ; aucune mise à jour automatique." >&2
    exit 1
fi
if [[ -n "$(git -C "$source_dir" status --porcelain --untracked-files=no)" ]]; then
    echo "Les sources llama.cpp ont des modifications locales ; compilation interrompue." >&2
    exit 1
fi

if [[ ! -x "$tools_dir/bin/python" ]]; then
    python3 -m venv "$tools_dir"
fi
if ! "$tools_dir/bin/python" -c 'import importlib.metadata,sys; sys.exit(importlib.metadata.version("cmake") != sys.argv[1])' "$cmake_version" 2>/dev/null; then
    "$tools_dir/bin/python" -m pip --disable-pip-version-check install --no-cache-dir "cmake==$cmake_version"
fi
cmake_bin="$tools_dir/bin/cmake"

"$cmake_bin" -S "$source_dir" -B "$build_dir" -G "Unix Makefiles" \
    -DCMAKE_BUILD_TYPE=Release \
    -DLLAMA_BUILD_IS_DEV=OFF \
    -DBUILD_SHARED_LIBS=OFF \
    -DLLAMA_BUILD_TESTS=OFF \
    -DLLAMA_BUILD_EXAMPLES=OFF \
    -DLLAMA_BUILD_TOOLS=ON \
    -DLLAMA_BUILD_SERVER=ON \
    -DLLAMA_BUILD_APP=OFF \
    -DLLAMA_BUILD_UI=OFF \
    -DLLAMA_USE_PREBUILT_UI=OFF \
    -DLLAMA_OPENSSL=OFF \
    -DGGML_NATIVE=OFF \
    -DGGML_OPENMP=OFF \
    -DGGML_METAL="$metal" \
    -DGGML_METAL_EMBED_LIBRARY=ON
"$cmake_bin" --build "$build_dir" --config Release --target llama-server --parallel "$build_jobs"

# Publish only after the compiled binary can execute. Static ggml/llama linking
# and embedded Metal code avoid workspace-dependent dylibs and shader files.
"$build_dir/bin/llama-server" --version
staged_binary=$(mktemp "$output_dir/.llama-server.XXXXXX")
trap 'rm -f "$staged_binary"' EXIT
install -m 755 "$build_dir/bin/llama-server" "$staged_binary"
mv -f "$staged_binary" "$output_dir/llama-server"
install -m 644 "$source_dir/LICENSE" "$output_dir/llama.cpp.LICENSE"
"$tools_dir/bin/python" - "$output_dir" "$llama_release" "$llama_commit" "$cmake_version" "$metal" <<'PY'
import hashlib
import json
import pathlib
import platform
import sys

output, release, commit, cmake, metal = sys.argv[1:]
folder = pathlib.Path(output)
binary = folder / "llama-server"
manifest = {
    "upstream": "https://github.com/ggml-org/llama.cpp",
    "release": release,
    "commit": commit,
    "cmake": cmake,
    "system": platform.system(),
    "architecture": platform.machine(),
    "configuration": "Release",
    "metal": metal == "ON",
    "shared_libraries": False,
    "sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
}
(folder / "llama-server.build.json").write_text(
    json.dumps(manifest, indent=2) + "\n", encoding="utf-8"
)
PY
echo "llama-server compilé : $output_dir/llama-server"
