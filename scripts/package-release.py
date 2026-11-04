#!/usr/bin/env python3
"""Package only explicit release inputs, never workspace saves or model weights."""

import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile


ROOT = Path(__file__).resolve().parent.parent
GO = os.environ.get("GO", "go")


def go(*args):
    env = dict(os.environ, CGO_ENABLED="0")
    return subprocess.check_output([GO, *args], cwd=ROOT, env=env, text=True)


def configuration():
    version = os.environ.get("VERSION", "dev")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._+-]{0,63}", version):
        raise ValueError("VERSION doit contenir 1 à 64 lettres, chiffres ou caractères . _ + -.")
    target_os, target_arch, host_os, host_arch = go(
        "env", "GOOS", "GOARCH", "GOHOSTOS", "GOHOSTARCH"
    ).splitlines()
    with_llama = os.environ.get("WITH_LLAMA", "1")
    if with_llama not in ("0", "1"):
        raise ValueError("WITH_LLAMA doit valoir 0 ou 1.")
    if with_llama == "1" and (
        target_os not in ("darwin", "linux")
        or (target_os, target_arch) != (host_os, host_arch)
    ):
        raise ValueError(
            "Le runtime llama-server se compile sur sa plateforme cible (macOS ou Linux). "
            "Utilisez WITH_LLAMA=0 pour une compilation Go vers une autre plateforme."
        )
    return version, target_os, target_arch, with_llama == "1"


def digest(path):
    result = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            result.update(block)
    return result.hexdigest()


def copy(source, destination, executable=False):
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, destination)
    destination.chmod(0o755 if executable else 0o644)


def license_files(folder):
    return sorted(
        path for path in folder.iterdir()
        if path.is_file() and path.name.upper().startswith(("LICENSE", "LICENCE", "COPYING", "NOTICE", "COPYRIGHT"))
    )


def go_licenses(package):
    # Package dependencies select the modules actually linked for GOOS/GOARCH.
    stream = go("list", "-mod=readonly", "-deps", "-json", "./cmd/aventure")
    decoder = json.JSONDecoder()
    modules = {}
    while stream.strip():
        entry, end = decoder.raw_decode(stream.lstrip())
        stream = stream.lstrip()[end:]
        module = entry.get("Module")
        if module and not module.get("Main"):
            source = module.get("Replace", module)
            modules[module["Path"]] = source
    goroot = Path(go("env", "GOROOT").strip())
    copy(goroot / "LICENSE", package / "licenses/go/LICENSE")
    notices = ["# Dépendances de la distribution", "", "- Go : voir `licenses/go/LICENSE`."]
    for name, module in sorted(modules.items()):
        folder = Path(module["Dir"])
        files = license_files(folder)
        if not files:
            raise ValueError(f"Aucun fichier de licence trouvé pour {name}.")
        target = Path("licenses/modules") / name / module.get("Version", "local")
        for source in files:
            copy(source, package / target / source.name)
        notices.append(f"- {name} {module.get('Version', '')} : `{target.as_posix()}/`.")
    return modules, notices


def native_runtime(package, target_os, target_arch):
    runtime = ROOT / "runtime"
    manifest = json.loads((runtime / "llama-server.build.json").read_text())
    system = {"darwin": "Darwin", "linux": "Linux"}[target_os]
    architectures = {"arm64": {"arm64", "aarch64"}, "amd64": {"x86_64", "AMD64"}}
    if (
        manifest.get("system") != system
        or manifest.get("architecture") not in architectures.get(target_arch, {target_arch})
        or manifest.get("shared_libraries") is not False
        or manifest.get("sha256") != digest(runtime / "llama-server")
    ):
        raise ValueError("Le manifeste du runtime ne correspond pas au binaire ou à la plateforme cible.")
    for name in ("llama-server", "llama-server.build.json", "llama.cpp.LICENSE"):
        copy(runtime / name, package / "runtime" / name, executable=name == "llama-server")
    # Preserve upstream notices alongside the statically linked native runtime.
    upstream = ROOT / "third_party/llama.cpp"
    for source in sorted(upstream.rglob("*")):
        relative = source.relative_to(upstream)
        if ".git" in relative.parts or not source.is_file():
            continue
        if source.name.upper().startswith(("LICENSE", "LICENCE", "COPYING", "NOTICE", "COPYRIGHT")) or relative.parts[0] == "licenses":
            copy(source, package / "licenses/llama.cpp" / relative)
    return manifest


def package_release(config):
    version, target_os, target_arch, with_llama = config
    name = f"ai-venture-{version}-{target_os}-{target_arch}"
    if not with_llama:
        name += "-game-only"
    executable = "aventure.exe" if target_os == "windows" else "aventure"
    dist = ROOT / "dist"
    dist.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".package-", dir=dist) as temp:
        staging = Path(temp)
        package = staging / name
        binary = package / executable
        copy(ROOT / "bin" / executable, binary, executable=True)
        settings = {}
        for line in go("version", "-m", str(binary)).splitlines():
            fields = line.split()
            if len(fields) == 2 and fields[0] == "build" and "=" in fields[1]:
                key, value = fields[1].split("=", 1)
                settings[key] = value
        if any(settings.get(key) != value for key, value in {
            "GOOS": target_os, "GOARCH": target_arch, "CGO_ENABLED": "0",
        }.items()):
            raise ValueError("Le binaire Go ne correspond pas à la cible sans CGO ; relancez make dist.")
        copy(ROOT / "docs/distribution.md", package / "README.md")
        for source in license_files(ROOT):
            copy(source, package / source.name)
        models = package / "models"
        models.mkdir()
        (models / "README.txt").write_text(
            "Placez ici un modèle GGUF, puis lancez :\n"
            "../aventure -provider llamacpp -gguf ./modele.gguf -lang fr\n"
            "Consultez le README de la distribution pour les autres modes.\n",
            encoding="utf-8",
        )
        modules, notices = go_licenses(package)
        native = native_runtime(package, target_os, target_arch) if with_llama else None
        if native:
            notices.append("- llama.cpp et ses dépendances natives : `runtime/llama.cpp.LICENSE` et `licenses/llama.cpp/`.")
        (package / "THIRD_PARTY_NOTICES.md").write_text("\n".join(notices) + "\n", encoding="utf-8")
        manifest = {
            "name": "ai-venture", "version": version,
            "os": target_os, "architecture": target_arch,
            "go": go("version").strip(), "cgo": False,
            "executable": executable, "sha256": digest(binary),
            "modules": {name: module.get("Version", "local") for name, module in sorted(modules.items())},
            "llama_server": native,
        }
        (package / "BUILD.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
        archive_name = name + ".tar.gz"
        archive = staging / archive_name
        # Stable ordering and metadata; no absolute workspace paths in the tar.
        with archive.open("wb") as raw, gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=0) as zipped:
            with tarfile.open(fileobj=zipped, mode="w") as tar:
                for path in [package, *sorted(package.rglob("*"))]:
                    info = tar.gettarinfo(str(path), arcname=str(path.relative_to(staging)))
                    info.uid = info.gid = info.mtime = 0
                    info.uname = info.gname = ""
                    info.mode = 0o755 if path.is_dir() or path == binary or path == package / "runtime/llama-server" else 0o644
                    if path.is_file():
                        with path.open("rb") as source:
                            tar.addfile(info, source)
                    else:
                        tar.addfile(info)
        checksum = staging / (archive_name + ".sha256")
        checksum.write_text(f"{digest(archive)}  {archive_name}\n", encoding="utf-8")
        os.replace(archive, dist / archive.name)
        os.replace(checksum, dist / checksum.name)
        print(f"Distribution : {dist / archive_name}")
        print(f"SHA-256      : {dist / checksum.name}")


if __name__ == "__main__":
    try:
        config = configuration()
        if sys.argv[1:] == ["--check"]:
            pass
        elif not sys.argv[1:]:
            package_release(config)
        else:
            raise ValueError("Usage : package-release.py [--check]")
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as error:
        print(f"Création de la distribution impossible : {error}", file=sys.stderr)
        sys.exit(1)
