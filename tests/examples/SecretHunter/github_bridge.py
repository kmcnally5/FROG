#!/usr/bin/env python3
"""
SecretHunter GitHub Bridge
Speaks the kLex native bridge protocol over stdin/stdout (line-delimited JSON).

Required:  pip install PyGitHub
Optional:  git in PATH (for deep/blobless clone mode)

Protocol:
  receive (stdin):  {"id": N, "fn": "name", "args": [...]}
  send   (stdout):  {"id": N, "result": value}
                    {"id": N, "error": "message"}
"""
import json
import os
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
import urllib.error


# ── Handler registry ──────────────────────────────────────────────────────────

HANDLERS = {}

def _handler(name):
    def decorator(fn):
        HANDLERS[name] = fn
        return fn
    return decorator


# ── check_deps ────────────────────────────────────────────────────────────────

@_handler("check_deps")
def check_deps():
    """Verify runtime dependencies. Returns a status dict."""
    result = {
        "python":        sys.version,
        "ok":            True,
        "missing":       [],
        "git_available": shutil.which("git") is not None,
    }
    try:
        import github
        result["pygithub"] = github.__version__
    except ImportError:
        result["ok"] = False
        result["missing"].append("PyGitHub  →  pip install PyGitHub")
    if not result["git_available"]:
        result["git_warning"] = "git not found in PATH — deep scan unavailable"
    return result


# ── list_repos ────────────────────────────────────────────────────────────────

@_handler("list_repos")
def list_repos(org_or_user, token, include_private=True):
    """
    List all repos in a GitHub org or user account.
    Returns {"repos": [...], "count": N, "error": null|string}
    """
    try:
        from github import Github, GithubException
    except ImportError:
        return {"repos": [], "count": 0, "error": "PyGitHub not installed — pip install PyGitHub"}

    g = Github(token) if token else Github()
    repos = []
    try:
        try:
            entity = g.get_organization(org_or_user)
        except GithubException:
            entity = g.get_user(org_or_user)

        for repo in entity.get_repos():
            if repo.private and not include_private:
                continue
            repos.append({
                "name":           repo.name,
                "full_name":      repo.full_name,
                "clone_url":      repo.clone_url,
                "description":    repo.description or "",
                "private":        repo.private,
                "size_kb":        repo.size,
                "default_branch": repo.default_branch,
            })
    except GithubException as e:
        return {"repos": [], "count": 0, "error": str(e)}

    return {"repos": repos, "count": len(repos), "error": None}


# ── fetch_tarball ─────────────────────────────────────────────────────────────

@_handler("fetch_tarball")
def fetch_tarball(full_name, token, out_dir):
    """
    Download the repo as a gzipped tarball (1 API call) and extract to out_dir.
    No git history — current files only. Fast and rate-limit friendly.
    Returns {"path": out_dir, "files_written": N, "error": null|string}
    """
    os.makedirs(out_dir, exist_ok=True)
    tmp_tar = os.path.join(out_dir, "_sh_download.tar.gz")

    try:
        url = f"https://api.github.com/repos/{full_name}/tarball/HEAD"
        headers = {"Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28"}
        if token:
            headers["Authorization"] = f"Bearer {token}"

        req = urllib.request.Request(url, headers=headers)
        with urllib.request.urlopen(req, timeout=120) as resp:
            with open(tmp_tar, "wb") as f:
                while True:
                    chunk = resp.read(65536)
                    if not chunk:
                        break
                    f.write(chunk)

        with tarfile.open(tmp_tar, "r:gz") as tar:
            members = tar.getmembers()
            if not members:
                return {"path": out_dir, "files_written": 0, "error": "empty tarball"}
            # GitHub tarballs have a top-level dir like owner-repo-sha/ — strip it
            prefix = members[0].name.split("/")[0] + "/"
            safe_members = []
            for m in members:
                if m.name.startswith(prefix):
                    m.name = m.name[len(prefix):]
                # Skip blank names, absolute paths, and path traversal attempts
                if not m.name or m.name.startswith("/") or ".." in m.name.split("/"):
                    continue
                safe_members.append(m)
            tar.extractall(out_dir, members=safe_members)

        os.remove(tmp_tar)
        file_count = sum(len(fnames) for _, _, fnames in os.walk(out_dir))
        return {"path": out_dir, "files_written": file_count, "error": None}

    except urllib.error.HTTPError as e:
        return {"path": out_dir, "files_written": 0, "error": f"HTTP {e.code}: {e.reason}"}
    except Exception as e:
        return {"path": out_dir, "files_written": 0, "error": str(e)}
    finally:
        if os.path.exists(tmp_tar):
            try:
                os.remove(tmp_tar)
            except OSError:
                pass


# ── clone_blobless ────────────────────────────────────────────────────────────

@_handler("clone_blobless")
def clone_blobless(clone_url, token, out_dir):
    """
    Blobless git clone: fetches all commit history and tree metadata but only
    downloads file blobs on demand. Far smaller than a full clone.
    Returns {"path": out_dir, "error": null|string}
    """
    if not shutil.which("git"):
        return {"path": "", "error": "git not found in PATH"}

    if os.path.exists(out_dir):
        shutil.rmtree(out_dir)

    # Inject token into HTTPS URL when provided
    auth_url = clone_url
    if token and clone_url.startswith("https://"):
        auth_url = clone_url.replace("https://", f"https://x-access-token:{token}@", 1)

    try:
        result = subprocess.run(
            ["git", "clone", "--filter=blob:none", "--no-single-branch", auth_url, out_dir],
            capture_output=True, text=True, timeout=600,
        )
        if result.returncode != 0:
            # Scrub token from any error message before returning
            err = result.stderr.strip()
            if token:
                err = err.replace(token, "***")
            return {"path": "", "error": err}
        return {"path": out_dir, "error": None}

    except subprocess.TimeoutExpired:
        return {"path": "", "error": "clone timed out after 10 minutes"}
    except Exception as e:
        return {"path": "", "error": str(e)}


# ── cleanup ───────────────────────────────────────────────────────────────────

@_handler("cleanup")
def cleanup(dir_path):
    """Remove a temp directory. Safety check: only removes paths under tmp."""
    if not dir_path:
        return {"ok": True}
    tmp_base = tempfile.gettempdir()
    real = os.path.realpath(dir_path)
    if not real.startswith(os.path.realpath(tmp_base)):
        return {"ok": False, "error": f"refused: {dir_path} is not under {tmp_base}"}
    try:
        if os.path.exists(real):
            shutil.rmtree(real)
        return {"ok": True, "error": None}
    except Exception as e:
        return {"ok": False, "error": str(e)}


@_handler("cleanup_stale")
def cleanup_stale(prefix="secrethunter_"):
    """Remove leftover temp dirs from any previously crashed scan."""
    tmp_base = tempfile.gettempdir()
    removed = []
    try:
        for entry in os.listdir(tmp_base):
            if entry.startswith(prefix):
                full = os.path.join(tmp_base, entry)
                if os.path.isdir(full):
                    shutil.rmtree(full, ignore_errors=True)
                    removed.append(full)
    except Exception:
        pass
    return {"removed": removed, "count": len(removed)}


# ── config management ─────────────────────────────────────────────────────────

@_handler("load_config")
def load_config(path):
    """
    Read a key=value config file. Returns a hash of all key/value pairs.
    Missing file returns an empty hash (not an error).
    """
    path = os.path.expanduser(path)
    if not os.path.exists(path):
        return {}
    try:
        cfg = {}
        with open(path, "r") as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                eq = line.find("=")
                if eq < 0:
                    continue
                key = line[:eq].strip()
                val = line[eq + 1:].strip()
                cfg[key] = val
        return cfg
    except Exception as e:
        return {"_error": str(e)}


@_handler("save_config")
def save_config(path, updates):
    """
    Merge updates (a dict) into the config file at path.
    Creates the file and parent directories if they do not exist.
    """
    path = os.path.expanduser(path)
    os.makedirs(os.path.dirname(path), exist_ok=True)

    # Read existing config
    existing = {}
    if os.path.exists(path):
        try:
            with open(path, "r") as f:
                for line in f:
                    line = line.strip()
                    if not line or line.startswith("#"):
                        continue
                    eq = line.find("=")
                    if eq >= 0:
                        existing[line[:eq].strip()] = line[eq + 1:].strip()
        except Exception:
            pass

    existing.update(updates)

    try:
        with open(path, "w") as f:
            f.write("# SecretHunter configuration\n")
            f.write("# Edit here or use the settings panel inside the app.\n\n")
            for k, v in existing.items():
                f.write(f"{k}={v}\n")
        return {"ok": True, "error": None}
    except Exception as e:
        return {"ok": False, "error": str(e)}


# ── Bridge loop ───────────────────────────────────────────────────────────────

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    req = None
    try:
        req     = json.loads(line)
        req_id  = req["id"]
        fn_name = req["fn"]
        args    = req.get("args", [])

        if fn_name not in HANDLERS:
            resp = {"id": req_id, "error": f"unknown function: {fn_name}"}
        else:
            result = HANDLERS[fn_name](*args)
            resp   = {"id": req_id, "result": result}

    except Exception as e:
        resp = {"id": req.get("id", 0) if req else 0, "error": str(e)}

    print(json.dumps(resp), flush=True)
