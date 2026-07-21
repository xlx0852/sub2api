//! Load base_url / API key from Codex `config.toml` + `auth.json`.
//!
//! Priority for credentials is decided by the caller. This module only extracts
//! values from the Codex home layout:
//!
//! ```text
//! $CODEX_HOME/config.toml
//! $CODEX_HOME/auth.json
//! ```

use std::collections::HashMap;
use std::env;
use std::fs;
use std::path::{Path, PathBuf};

use serde::Deserialize;
use serde_json::Value as JsonValue;

#[derive(Debug, Clone, Default)]
pub struct CodexCredentials {
    pub base_url: Option<String>,
    pub api_key: Option<String>,
    pub provider_id: Option<String>,
    pub source: Option<String>,
}

#[derive(Debug, Deserialize)]
struct CodexConfigToml {
    #[serde(default)]
    model_provider: Option<String>,
    #[serde(default)]
    model_providers: HashMap<String, ModelProviderToml>,
}

#[derive(Debug, Deserialize, Default)]
struct ModelProviderToml {
    #[serde(default)]
    name: Option<String>,
    #[serde(default)]
    base_url: Option<String>,
    /// Codex Desktop / custom provider bearer token.
    #[serde(default)]
    experimental_bearer_token: Option<String>,
    /// Alternate key field some configs use.
    #[serde(default)]
    api_key: Option<String>,
    /// Env var name that holds the key (Codex style).
    #[serde(default)]
    env_key: Option<String>,
    #[serde(default)]
    #[allow(dead_code)]
    wire_api: Option<String>,
}

pub fn default_codex_home() -> PathBuf {
    if let Ok(home) = env::var("CODEX_HOME") {
        let home = home.trim();
        if !home.is_empty() {
            return PathBuf::from(home);
        }
    }
    dirs_home()
        .map(|h| h.join(".codex"))
        .unwrap_or_else(|| PathBuf::from(".codex"))
}

fn dirs_home() -> Option<PathBuf> {
    env::var_os("HOME")
        .or_else(|| env::var_os("USERPROFILE"))
        .map(PathBuf::from)
}

/// Load credentials from Codex config.
///
/// `provider_override`: explicit provider id (`OpenAI`, `sicts`, …).
/// When empty, uses top-level `model_provider`.
pub fn load_codex_credentials(
    codex_home: &Path,
    provider_override: Option<&str>,
) -> Result<CodexCredentials, String> {
    let config_path = codex_home.join("config.toml");
    let auth_path = codex_home.join("auth.json");

    let mut out = CodexCredentials::default();
    if !config_path.is_file() {
        // Still try auth.json alone.
        out.api_key = load_auth_api_key(&auth_path);
        if out.api_key.is_some() {
            out.source = Some(format!("auth.json ({})", auth_path.display()));
        }
        return Ok(out);
    }

    let text = fs::read_to_string(&config_path)
        .map_err(|e| format!("failed to read {}: {e}", config_path.display()))?;
    let cfg: CodexConfigToml = toml::from_str(&text)
        .map_err(|e| format!("failed to parse {}: {e}", config_path.display()))?;

    let provider_id = provider_override
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map(|s| s.to_string())
        .or_else(|| {
            cfg.model_provider
                .as_deref()
                .map(str::trim)
                .filter(|s| !s.is_empty())
                .map(|s| s.to_string())
        });

    let resolved = resolve_provider(&cfg, provider_id.as_deref(), provider_override.is_some());

    if let Some((id, p)) = resolved {
        out.base_url = p
            .base_url
            .as_deref()
            .map(str::trim)
            .filter(|s| !s.is_empty())
            .map(|s| s.to_string());

        // Token preference inside provider block.
        out.api_key = p
            .experimental_bearer_token
            .as_deref()
            .map(str::trim)
            .filter(|s| !s.is_empty())
            .map(|s| s.to_string())
            .or_else(|| {
                p.api_key
                    .as_deref()
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .map(|s| s.to_string())
            })
            .or_else(|| {
                p.env_key.as_deref().and_then(|name| {
                    let name = name.trim();
                    if name.is_empty() {
                        return None;
                    }
                    env::var(name).ok().and_then(|v| {
                        let v = v.trim().to_string();
                        if v.is_empty() {
                            None
                        } else {
                            Some(v)
                        }
                    })
                })
            });

        out.provider_id = Some(id);
        out.source = Some(format!(
            "config.toml provider={} ({})",
            out.provider_id.as_deref().unwrap_or("?"),
            config_path.display()
        ));
    } else if let Some(id) = provider_id {
        // Provider id set but missing — keep id for error messages.
        out.provider_id = Some(id);
        out.source = Some(format!("config.toml ({})", config_path.display()));
    } else {
        out.source = Some(format!("config.toml ({})", config_path.display()));
    }

    // auth.json fallback for key only.
    if out.api_key.is_none() {
        if let Some(k) = load_auth_api_key(&auth_path) {
            out.api_key = Some(k);
            let prev = out.source.take().unwrap_or_default();
            out.source = Some(if prev.is_empty() {
                format!("auth.json ({})", auth_path.display())
            } else {
                format!("{prev} + auth.json")
            });
        }
    }

    Ok(out)
}

fn resolve_provider<'a>(
    cfg: &'a CodexConfigToml,
    provider_id: Option<&str>,
    strict: bool,
) -> Option<(String, &'a ModelProviderToml)> {
    if let Some(id) = provider_id {
        if let Some(p) = cfg.model_providers.get(id) {
            return Some((id.to_string(), p));
        }
        if let Some((k, p)) = cfg
            .model_providers
            .iter()
            .find(|(k, _)| k.eq_ignore_ascii_case(id))
        {
            return Some((k.clone(), p));
        }
        if let Some((k, p)) = cfg.model_providers.iter().find(|(_, v)| {
            v.name
                .as_deref()
                .is_some_and(|n| n.eq_ignore_ascii_case(id))
        }) {
            return Some((k.clone(), p));
        }
        if strict {
            return None;
        }
    }

    // No explicit/active match: pick first provider that has a base_url.
    if !strict {
        if let Some((k, p)) = cfg.model_providers.iter().find(|(_, p)| {
            p.base_url
                .as_deref()
                .is_some_and(|u| !u.trim().is_empty())
        }) {
            return Some((k.clone(), p));
        }
    }
    None
}

fn load_auth_api_key(path: &Path) -> Option<String> {
    if !path.is_file() {
        return None;
    }
    let text = fs::read_to_string(path).ok()?;
    let v: JsonValue = serde_json::from_str(&text).ok()?;
    for key in ["OPENAI_API_KEY", "SICTS_API_KEY", "api_key", "access_token"] {
        if let Some(s) = v.get(key).and_then(|x| x.as_str()) {
            let s = s.trim();
            if !s.is_empty() {
                return Some(s.to_string());
            }
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;

    #[test]
    fn parses_active_provider() {
        let dir = tempfile_dir();
        let cfg = r#"
model_provider = "OpenAI"

[model_providers.OpenAI]
name = "OpenAI"
base_url = "https://code.sicts.shop"
experimental_bearer_token = "sk-test-123"
wire_api = "responses"
"#;
        fs::write(dir.join("config.toml"), cfg).unwrap();
        let creds = load_codex_credentials(&dir, None).unwrap();
        assert_eq!(creds.base_url.as_deref(), Some("https://code.sicts.shop"));
        assert_eq!(creds.api_key.as_deref(), Some("sk-test-123"));
        assert_eq!(creds.provider_id.as_deref(), Some("OpenAI"));
    }

    #[test]
    fn auth_json_fallback() {
        let dir = tempfile_dir();
        fs::write(
            dir.join("config.toml"),
            r#"
model_provider = "sicts"
[model_providers.sicts]
base_url = "https://code.sicts.shop/v1"
"#,
        )
        .unwrap();
        fs::write(
            dir.join("auth.json"),
            r#"{"OPENAI_API_KEY":"sk-from-auth"}"#,
        )
        .unwrap();
        let creds = load_codex_credentials(&dir, None).unwrap();
        assert_eq!(creds.api_key.as_deref(), Some("sk-from-auth"));
        assert!(creds.base_url.as_deref().unwrap().contains("code.sicts.shop"));
    }

    fn tempfile_dir() -> PathBuf {
        use std::sync::atomic::{AtomicU64, Ordering};
        static COUNTER: AtomicU64 = AtomicU64::new(0);
        let n = COUNTER.fetch_add(1, Ordering::Relaxed);
        let mut path = env::temp_dir();
        path.push(format!(
            "sicts-image-gen-test-{}-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos(),
            n
        ));
        fs::create_dir_all(&path).unwrap();
        let mut f = fs::File::create(path.join(".keep")).unwrap();
        let _ = f.write_all(b"1");
        path
    }
}
