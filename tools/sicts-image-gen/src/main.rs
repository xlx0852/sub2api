//! Lightweight Images API CLI for SicTs / Sub2API.
//! Replaces Codex built-in `image_gen` when the host tool is not mounted
//! (API-key / custom base_url sessions).

mod codex_config;

use std::env;
use std::fs;
use std::io::{self, Read};
use std::path::{Path, PathBuf};
use std::process::{Command, ExitCode};
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::Duration;

use base64::Engine;
use clap::{Parser, Subcommand, ValueEnum};
use reqwest::blocking::multipart;
use reqwest::header::{AUTHORIZATION, CONTENT_TYPE, HeaderMap, HeaderValue};
use serde::Deserialize;
use serde_json::{json, Value};

use codex_config::{default_codex_home, load_codex_credentials};

const DEFAULT_BASE_URL: &str = "https://code.sicts.shop/v1";
const DEFAULT_MODEL: &str = "gpt-image-2";
const DEFAULT_SIZE: &str = "auto";
const DEFAULT_QUALITY: &str = "medium";
const DEFAULT_OUT: &str = "output/imagegen/output.png";
const MAX_IMAGE_BYTES: u64 = 50 * 1024 * 1024;
const REQUEST_TIMEOUT_SECS: u64 = 900;
const DEFAULT_BATCH_CONCURRENCY: usize = 4;
const MAX_BATCH_CONCURRENCY: usize = 16;

#[derive(Parser, Debug)]
#[command(
    name = "sicts-image-gen",
    about = "SicTs / Sub2API Images API CLI (Codex image_gen fallback)",
    version
)]
struct Cli {
    /// Gateway base URL ending with /v1 (env: SICTS_BASE_URL or OPENAI_BASE_URL).
    /// Falls back to Codex config.toml model_providers.*.base_url.
    #[arg(long, global = true, env = "SICTS_BASE_URL")]
    base_url: Option<String>,

    /// API key (env: SICTS_API_KEY or OPENAI_API_KEY).
    /// Falls back to Codex config.toml experimental_bearer_token / env_key, then auth.json.
    #[arg(long, global = true, env = "SICTS_API_KEY")]
    api_key: Option<String>,

    /// Codex home directory (default: $CODEX_HOME or ~/.codex)
    #[arg(long, global = true, env = "CODEX_HOME")]
    codex_home: Option<PathBuf>,

    /// Codex model_provider id to read from config.toml (default: active model_provider)
    #[arg(long, global = true, env = "SICTS_CODEX_PROVIDER")]
    provider: Option<String>,

    /// Do not read ~/.codex/config.toml or auth.json
    #[arg(long, global = true, default_value_t = false)]
    no_codex_config: bool,

    /// Print resolved base_url / provider / key source to stderr
    #[arg(long, global = true, default_value_t = false)]
    print_config: bool,

    /// Print request payload only; do not call the API
    #[arg(long, global = true, default_value_t = false)]
    dry_run: bool,

    /// Request timeout in seconds
    #[arg(long, global = true, default_value_t = REQUEST_TIMEOUT_SECS)]
    timeout: u64,

    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand, Debug)]
enum Commands {
    /// Text-to-image via POST /images/generations
    Generate {
        #[arg(long)]
        prompt: Option<String>,

        #[arg(long)]
        prompt_file: Option<PathBuf>,

        #[arg(long, default_value = DEFAULT_MODEL)]
        model: String,

        #[arg(long, default_value = DEFAULT_SIZE)]
        size: String,

        #[arg(long, default_value = DEFAULT_QUALITY)]
        quality: String,

        #[arg(long, default_value_t = 1)]
        n: u8,

        #[arg(long, value_enum, default_value_t = OutputFormat::Png)]
        output_format: OutputFormat,

        #[arg(long)]
        background: Option<Background>,

        #[arg(long)]
        moderation: Option<String>,

        #[arg(long, default_value = DEFAULT_OUT)]
        out: PathBuf,

        #[arg(long)]
        out_dir: Option<PathBuf>,
    },

    /// Image edit via POST /images/edits (multipart)
    Edit {
        #[arg(long)]
        prompt: Option<String>,

        #[arg(long)]
        prompt_file: Option<PathBuf>,

        /// One or more input images
        #[arg(long = "image", required = true)]
        images: Vec<PathBuf>,

        #[arg(long)]
        mask: Option<PathBuf>,

        #[arg(long, default_value = DEFAULT_MODEL)]
        model: String,

        #[arg(long, default_value = DEFAULT_SIZE)]
        size: String,

        #[arg(long, default_value = DEFAULT_QUALITY)]
        quality: String,

        #[arg(long, default_value_t = 1)]
        n: u8,

        #[arg(long, value_enum, default_value_t = OutputFormat::Png)]
        output_format: OutputFormat,

        #[arg(long)]
        background: Option<Background>,

        #[arg(long)]
        input_fidelity: Option<InputFidelity>,

        #[arg(long, default_value = DEFAULT_OUT)]
        out: PathBuf,

        #[arg(long)]
        out_dir: Option<PathBuf>,
    },

    /// Batch generate from JSONL (one JSON object per line with "prompt")
    GenerateBatch {
        #[arg(long)]
        input: PathBuf,

        #[arg(long, default_value = "output/imagegen/batch")]
        out_dir: PathBuf,

        #[arg(long, default_value = DEFAULT_MODEL)]
        model: String,

        #[arg(long, default_value = DEFAULT_SIZE)]
        size: String,

        #[arg(long, default_value = DEFAULT_QUALITY)]
        quality: String,

        #[arg(long, value_enum, default_value_t = OutputFormat::Png)]
        output_format: OutputFormat,

        /// Max parallel API requests (1 = serial). Env: SICTS_BATCH_CONCURRENCY
        #[arg(long, default_value_t = DEFAULT_BATCH_CONCURRENCY, env = "SICTS_BATCH_CONCURRENCY")]
        concurrency: usize,
    },

    /// Remove flat chroma-key background (local Pillow helper; no API call)
    RemoveChroma {
        #[arg(long)]
        input: PathBuf,

        #[arg(long)]
        out: PathBuf,

        /// Hex RGB key color, e.g. #00ff00 (ignored when --auto-key is set)
        #[arg(long, default_value = "#00ff00")]
        key_color: String,

        /// Sample key from image: none|corners|border
        #[arg(long, default_value = "border")]
        auto_key: String,

        #[arg(long, default_value_t = 12)]
        tolerance: u32,

        /// Soft edge matte (recommended). Accepts `--soft-matte` or `--soft-matte true|false`.
        #[arg(
            long,
            default_value_t = true,
            num_args = 0..=1,
            default_missing_value = "true",
            action = clap::ArgAction::Set
        )]
        soft_matte: bool,

        #[arg(long, default_value_t = 12.0)]
        transparent_threshold: f32,

        #[arg(long, default_value_t = 220.0)]
        opaque_threshold: f32,

        #[arg(long, default_value_t = 0.0)]
        edge_feather: f32,

        #[arg(long, default_value_t = 0)]
        edge_contract: u32,

        /// Decontaminate key-color spill on edges. Accepts `--despill` or `--despill true|false`.
        #[arg(
            long,
            default_value_t = true,
            num_args = 0..=1,
            default_missing_value = "true",
            action = clap::ArgAction::Set
        )]
        despill: bool,

        /// Overwrite existing --out
        #[arg(long, default_value_t = false)]
        force: bool,
    },
}

#[derive(Clone, Copy, Debug, ValueEnum)]
enum OutputFormat {
    Png,
    Jpeg,
    Webp,
}

impl OutputFormat {
    fn as_str(self) -> &'static str {
        match self {
            Self::Png => "png",
            Self::Jpeg => "jpeg",
            Self::Webp => "webp",
        }
    }

    fn extension(self) -> &'static str {
        self.as_str()
    }
}

#[derive(Clone, Copy, Debug, ValueEnum)]
enum Background {
    Transparent,
    Opaque,
    Auto,
}

impl Background {
    fn as_str(self) -> &'static str {
        match self {
            Self::Transparent => "transparent",
            Self::Opaque => "opaque",
            Self::Auto => "auto",
        }
    }
}

#[derive(Clone, Copy, Debug, ValueEnum)]
enum InputFidelity {
    Low,
    High,
}

impl InputFidelity {
    fn as_str(self) -> &'static str {
        match self {
            Self::Low => "low",
            Self::High => "high",
        }
    }
}

#[derive(Debug, Deserialize)]
struct ImagesResponse {
    data: Vec<ImageData>,
}

#[derive(Debug, Deserialize)]
struct ImageData {
    b64_json: Option<String>,
    url: Option<String>,
}

fn main() -> ExitCode {
    match run() {
        Ok(()) => ExitCode::SUCCESS,
        Err(err) => {
            eprintln!("Error: {err}");
            ExitCode::FAILURE
        }
    }
}

fn run() -> Result<(), String> {
    let cli = Cli::parse();
    let resolved = resolve_runtime_config(&cli)?;
    let base_url = resolved.base_url;
    let api_key = resolved.api_key;

    if cli.print_config || cli.dry_run {
        eprintln!("config: base_url={base_url}");
        if let Some(p) = &resolved.provider_id {
            eprintln!("config: provider={p}");
        }
        if let Some(s) = &resolved.source {
            eprintln!("config: source={s}");
        }
        eprintln!(
            "config: api_key={}",
            if api_key.is_empty() {
                "(empty)".to_string()
            } else {
                redact_key(&api_key)
            }
        );
    }

    match cli.command {
        Commands::Generate {
            prompt,
            prompt_file,
            model,
            size,
            quality,
            n,
            output_format,
            background,
            moderation,
            out,
            out_dir,
        } => {
            let prompt = resolve_prompt(prompt, prompt_file)?;
            validate_common(&model, &size, &quality, n, background, output_format)?;
            let mut body = json!({
                "model": model,
                "prompt": prompt,
                "n": n,
                "size": size,
                "quality": quality,
                "output_format": output_format.as_str(),
                "response_format": "b64_json",
            });
            if let Some(bg) = background {
                body["background"] = json!(bg.as_str());
            }
            if let Some(m) = moderation {
                body["moderation"] = json!(m);
            }

            if cli.dry_run {
                print_dry_run("POST", &format!("{base_url}/images/generations"), &body)?;
                return Ok(());
            }

            let resp = post_json(
                &base_url,
                "/images/generations",
                &api_key,
                &body,
                cli.timeout,
            )?;
            let paths = save_response(&resp, &out, out_dir.as_deref(), output_format, n as usize)?;
            for p in paths {
                println!("{}", p.display());
            }
            Ok(())
        }
        Commands::Edit {
            prompt,
            prompt_file,
            images,
            mask,
            model,
            size,
            quality,
            n,
            output_format,
            background,
            input_fidelity,
            out,
            out_dir,
        } => {
            let prompt = resolve_prompt(prompt, prompt_file)?;
            validate_common(&model, &size, &quality, n, background, output_format)?;
            if model == "gpt-image-2" && input_fidelity.is_some() {
                return Err(
                    "input_fidelity is not supported for gpt-image-2 (always high fidelity)"
                        .into(),
                );
            }
            for img in &images {
                check_image_file(img)?;
            }
            if let Some(m) = &mask {
                check_image_file(m)?;
            }

            if cli.dry_run {
                let mut body = json!({
                    "model": model,
                    "prompt": prompt,
                    "n": n,
                    "size": size,
                    "quality": quality,
                    "output_format": output_format.as_str(),
                    "response_format": "b64_json",
                    "images": images.iter().map(|p| p.display().to_string()).collect::<Vec<_>>(),
                });
                if let Some(bg) = background {
                    body["background"] = json!(bg.as_str());
                }
                if let Some(f) = input_fidelity {
                    body["input_fidelity"] = json!(f.as_str());
                }
                if let Some(m) = &mask {
                    body["mask"] = json!(m.display().to_string());
                }
                print_dry_run("POST", &format!("{base_url}/images/edits"), &body)?;
                return Ok(());
            }

            let resp = post_edit_multipart(
                &base_url,
                &api_key,
                &prompt,
                &images,
                mask.as_deref(),
                &model,
                &size,
                &quality,
                n,
                output_format,
                background,
                input_fidelity,
                cli.timeout,
            )?;
            let paths = save_response(&resp, &out, out_dir.as_deref(), output_format, n as usize)?;
            for p in paths {
                println!("{}", p.display());
            }
            Ok(())
        }
        Commands::GenerateBatch {
            input,
            out_dir,
            model,
            size,
            quality,
            output_format,
            concurrency,
        } => {
            validate_common(&model, &size, &quality, 1, None, output_format)?;
            if !(1..=MAX_BATCH_CONCURRENCY).contains(&concurrency) {
                return Err(format!(
                    "concurrency must be between 1 and {MAX_BATCH_CONCURRENCY} (got {concurrency})"
                ));
            }
            let text = fs::read_to_string(&input)
                .map_err(|e| format!("failed to read {}: {e}", input.display()))?;
            fs::create_dir_all(&out_dir)
                .map_err(|e| format!("failed to create {}: {e}", out_dir.display()))?;

            let mut jobs = Vec::new();
            for (line_no, line) in text.lines().enumerate() {
                let line = line.trim();
                if line.is_empty() {
                    continue;
                }
                let index = jobs.len() + 1;
                let obj: Value = serde_json::from_str(line)
                    .map_err(|e| format!("invalid JSONL at line {}: {e}", line_no + 1))?;
                let prompt = obj
                    .get("prompt")
                    .and_then(|v| v.as_str())
                    .ok_or_else(|| format!("line {}: missing prompt", line_no + 1))?
                    .to_string();
                let job_model = obj
                    .get("model")
                    .and_then(|v| v.as_str())
                    .unwrap_or(&model)
                    .to_string();
                let job_size = obj
                    .get("size")
                    .and_then(|v| v.as_str())
                    .unwrap_or(&size)
                    .to_string();
                let job_quality = obj
                    .get("quality")
                    .and_then(|v| v.as_str())
                    .unwrap_or(&quality)
                    .to_string();
                validate_common(
                    &job_model,
                    &job_size,
                    &job_quality,
                    1,
                    None,
                    output_format,
                )
                .map_err(|e| format!("line {}: {e}", line_no + 1))?;
                let out = out_dir.join(format!("image_{index:03}.{}", output_format.extension()));
                let body = json!({
                    "model": job_model,
                    "prompt": prompt,
                    "n": 1,
                    "size": job_size,
                    "quality": job_quality,
                    "output_format": output_format.as_str(),
                    "response_format": "b64_json",
                });
                jobs.push(BatchJob {
                    index,
                    line_no: line_no + 1,
                    out,
                    body,
                });
            }
            if jobs.is_empty() {
                return Err("JSONL input had no jobs".into());
            }

            if cli.dry_run {
                for job in &jobs {
                    println!("# job {} (line {})", job.index, job.line_no);
                    print_dry_run(
                        "POST",
                        &format!("{base_url}/images/generations"),
                        &job.body,
                    )?;
                }
                eprintln!(
                    "dry-run: {} jobs, concurrency would be {concurrency}",
                    jobs.len()
                );
                return Ok(());
            }

            let results = run_batch_parallel(
                &jobs,
                &base_url,
                &api_key,
                cli.timeout,
                output_format,
                concurrency,
            )?;
            let mut errors = Vec::new();
            for item in results {
                match item {
                    Ok(paths) => {
                        for p in paths {
                            println!("{}", p.display());
                        }
                    }
                    Err(e) => errors.push(e),
                }
            }
            if !errors.is_empty() {
                for e in &errors {
                    eprintln!("batch error: {e}");
                }
                return Err(format!(
                    "{} of {} batch jobs failed",
                    errors.len(),
                    jobs.len()
                ));
            }
            Ok(())
        }
        Commands::RemoveChroma {
            input,
            out,
            key_color,
            auto_key,
            tolerance,
            soft_matte,
            transparent_threshold,
            opaque_threshold,
            edge_feather,
            edge_contract,
            despill,
            force,
        } => {
            if cli.dry_run {
                println!(
                    "remove-chroma input={} out={} auto_key={} key_color={} soft_matte={} despill={}",
                    input.display(),
                    out.display(),
                    auto_key,
                    key_color,
                    soft_matte,
                    despill
                );
                return Ok(());
            }
            run_remove_chroma(
                &input,
                &out,
                &key_color,
                &auto_key,
                tolerance,
                soft_matte,
                transparent_threshold,
                opaque_threshold,
                edge_feather,
                edge_contract,
                despill,
                force,
                cli.codex_home.as_deref(),
            )
        }
    }
}

struct RuntimeConfig {
    base_url: String,
    api_key: String,
    provider_id: Option<String>,
    source: Option<String>,
}

/// Resolve order:
/// 1. CLI flags / process env already bound by clap (`--base-url`, `--api-key`)
/// 2. OPENAI_BASE_URL / OPENAI_API_KEY env (if not already via clap SICTS_*)
/// 3. Codex `$CODEX_HOME/config.toml` active provider (+ auth.json)
/// 4. Built-in default base_url
fn resolve_runtime_config(cli: &Cli) -> Result<RuntimeConfig, String> {
    let codex = if cli.no_codex_config {
        None
    } else {
        let home = cli
            .codex_home
            .clone()
            .unwrap_or_else(default_codex_home);
        match load_codex_credentials(&home, cli.provider.as_deref()) {
            Ok(c) => Some(c),
            Err(e) => {
                eprintln!("Warning: codex config: {e}");
                None
            }
        }
    };

    // base_url: clap already maps SICTS_BASE_URL into cli.base_url when present.
    let raw_base = cli
        .base_url
        .as_deref()
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map(|s| s.to_string())
        .or_else(|| env_nonempty("OPENAI_BASE_URL"))
        .or_else(|| codex.as_ref().and_then(|c| c.base_url.clone()))
        .unwrap_or_else(|| DEFAULT_BASE_URL.to_string());
    let base_url = normalize_base_url(&raw_base)?;

    // api_key: clap maps SICTS_API_KEY into cli.api_key.
    let api_key = cli
        .api_key
        .as_deref()
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .map(|s| s.to_string())
        .or_else(|| env_nonempty("OPENAI_API_KEY"))
        .or_else(|| codex.as_ref().and_then(|c| c.api_key.clone()))
        .unwrap_or_default();

    if api_key.is_empty() && !cli.dry_run {
        return Err(
            "API key missing. Set SICTS_API_KEY / OPENAI_API_KEY, pass --api-key, or configure Codex ~/.codex/config.toml experimental_bearer_token / auth.json."
                .into(),
        );
    }
    if api_key.is_empty() && cli.dry_run {
        eprintln!("Warning: no API key set; dry-run only.");
    }

    let mut source_parts = Vec::new();
    if cli.base_url.is_some() {
        source_parts.push("cli/env base_url".to_string());
    } else if env_nonempty("OPENAI_BASE_URL").is_some() {
        source_parts.push("OPENAI_BASE_URL".to_string());
    } else if codex
        .as_ref()
        .and_then(|c| c.base_url.as_ref())
        .is_some()
    {
        source_parts.push("codex config.toml base_url".to_string());
    } else {
        source_parts.push("default base_url".to_string());
    }
    if cli.api_key.is_some() {
        source_parts.push("cli/env api_key".to_string());
    } else if env_nonempty("OPENAI_API_KEY").is_some() {
        source_parts.push("OPENAI_API_KEY".to_string());
    } else if codex.as_ref().and_then(|c| c.api_key.as_ref()).is_some() {
        source_parts.push("codex key".to_string());
    }
    if let Some(c) = &codex {
        if let Some(s) = &c.source {
            source_parts.push(s.clone());
        }
    }

    Ok(RuntimeConfig {
        base_url,
        api_key,
        provider_id: codex.and_then(|c| c.provider_id),
        source: Some(source_parts.join(" | ")),
    })
}

fn env_nonempty(key: &str) -> Option<String> {
    env::var(key).ok().and_then(|v| {
        let v = v.trim().to_string();
        if v.is_empty() {
            None
        } else {
            Some(v)
        }
    })
}

fn normalize_base_url(raw: &str) -> Result<String, String> {
    let mut url = raw.trim().trim_end_matches('/').to_string();
    if url.is_empty() {
        return Err("base_url is empty".into());
    }
    // Accept gateway root without /v1.
    if !url.ends_with("/v1") {
        url.push_str("/v1");
    }
    Ok(url)
}

fn redact_key(key: &str) -> String {
    let k = key.trim();
    if k.len() <= 12 {
        return "***".into();
    }
    format!("{}…{}", &k[..6], &k[k.len() - 4..])
}

fn resolve_prompt(prompt: Option<String>, prompt_file: Option<PathBuf>) -> Result<String, String> {
    match (prompt, prompt_file) {
        (Some(_), Some(_)) => Err("use --prompt or --prompt-file, not both".into()),
        (Some(p), None) => {
            let p = p.trim().to_string();
            if p.is_empty() {
                Err("prompt is empty".into())
            } else {
                Ok(p)
            }
        }
        (None, Some(path)) => {
            let text = fs::read_to_string(&path)
                .map_err(|e| format!("failed to read prompt file {}: {e}", path.display()))?;
            let text = text.trim().to_string();
            if text.is_empty() {
                Err("prompt file is empty".into())
            } else {
                Ok(text)
            }
        }
        (None, None) => {
            // Allow piping prompt on stdin (e.g. `echo "..." | sicts-image-gen generate`).
            let mut buf = String::new();
            io::stdin()
                .read_to_string(&mut buf)
                .map_err(|e| format!("failed to read stdin: {e}"))?;
            let buf = buf.trim().to_string();
            if buf.is_empty() {
                Err("missing --prompt or --prompt-file".into())
            } else {
                Ok(buf)
            }
        }
    }
}

fn validate_common(
    model: &str,
    size: &str,
    quality: &str,
    n: u8,
    background: Option<Background>,
    output_format: OutputFormat,
) -> Result<(), String> {
    if !model.starts_with("gpt-image-") {
        return Err(format!(
            "model must be a GPT Image model (got {model}); examples: gpt-image-2, gpt-image-1.5"
        ));
    }
    if !(1..=10).contains(&n) {
        return Err("n must be between 1 and 10".into());
    }
    match quality {
        "low" | "medium" | "high" | "auto" => {}
        _ => return Err("quality must be low|medium|high|auto".into()),
    }
    validate_size(model, size)?;
    if let Some(Background::Transparent) = background {
        if model == "gpt-image-2" {
            return Err(
                "gpt-image-2 does not support background=transparent; use --model gpt-image-1.5 --background transparent --output-format png, or generate on chroma-key then: sicts-image-gen remove-chroma --input <src> --out <dst>"
                    .into(),
            );
        }
        match output_format {
            OutputFormat::Png | OutputFormat::Webp => {}
            OutputFormat::Jpeg => {
                return Err("transparent background requires output-format png or webp".into())
            }
        }
    }
    Ok(())
}

fn validate_size(model: &str, size: &str) -> Result<(), String> {
    if size == "auto" {
        return Ok(());
    }
    if model == "gpt-image-2" {
        let (w, h) = parse_wh(size)?;
        let max_edge = w.max(h);
        let min_edge = w.min(h);
        let total = w as u64 * h as u64;
        if max_edge > 3840 {
            return Err("gpt-image-2 max edge must be <= 3840".into());
        }
        if w % 16 != 0 || h % 16 != 0 {
            return Err("gpt-image-2 width/height must be multiples of 16".into());
        }
        if max_edge as f64 / min_edge as f64 > 3.0 {
            return Err("gpt-image-2 aspect ratio must be <= 3:1".into());
        }
        if !(655_360..=8_294_400).contains(&total) {
            return Err("gpt-image-2 total pixels must be in [655360, 8294400]".into());
        }
        return Ok(());
    }
    match size {
        "1024x1024" | "1536x1024" | "1024x1536" | "auto" => Ok(()),
        _ => Err(
            "legacy GPT Image size must be 1024x1024, 1536x1024, 1024x1536, or auto".into(),
        ),
    }
}

fn parse_wh(size: &str) -> Result<(u32, u32), String> {
    let parts: Vec<_> = size.split('x').collect();
    if parts.len() != 2 {
        return Err("size must be auto or WIDTHxHEIGHT".into());
    }
    let w: u32 = parts[0]
        .parse()
        .map_err(|_| "invalid size width".to_string())?;
    let h: u32 = parts[1]
        .parse()
        .map_err(|_| "invalid size height".to_string())?;
    if w == 0 || h == 0 {
        return Err("size dimensions must be > 0".into());
    }
    Ok((w, h))
}

fn check_image_file(path: &Path) -> Result<(), String> {
    let meta = fs::metadata(path).map_err(|e| format!("image not found {}: {e}", path.display()))?;
    if !meta.is_file() {
        return Err(format!("not a file: {}", path.display()));
    }
    if meta.len() > MAX_IMAGE_BYTES {
        eprintln!(
            "Warning: {} exceeds 50MB limit ({})",
            path.display(),
            meta.len()
        );
    }
    Ok(())
}

fn print_dry_run(method: &str, url: &str, body: &Value) -> Result<(), String> {
    println!("{method} {url}");
    println!(
        "{}",
        serde_json::to_string_pretty(body).map_err(|e| e.to_string())?
    );
    Ok(())
}

/// Locate bundled remove_chroma_key.py next to the skill/CLI install.
fn resolve_remove_chroma_script(codex_home: Option<&Path>) -> Result<PathBuf, String> {
    let mut candidates = Vec::new();

    // 1) Same directory as this executable (skill scripts/)
    if let Ok(exe) = env::current_exe() {
        if let Some(dir) = exe.parent() {
            candidates.push(dir.join("remove_chroma_key.py"));
            // repo layout: target/release -> ../../../skills/...
            candidates.push(
                dir.join("../../../skills/sicts-imagegen/scripts/remove_chroma_key.py"),
            );
        }
    }

    // 2) Installed Codex skill
    let home = codex_home
        .map(PathBuf::from)
        .unwrap_or_else(default_codex_home);
    candidates.push(home.join("skills/sicts-imagegen/scripts/remove_chroma_key.py"));

    // 3) Legacy system skill path (if still present)
    candidates.push(home.join("skills/.system/imagegen/scripts/remove_chroma_key.py"));

    // 4) env override
    if let Ok(p) = env::var("SICTS_REMOVE_CHROMA_SCRIPT") {
        candidates.insert(0, PathBuf::from(p));
    }

    for c in &candidates {
        if c.is_file() {
            return Ok(c.clone());
        }
    }
    Err(format!(
        "remove_chroma_key.py not found. Tried:\n  {}",
        candidates
            .iter()
            .map(|p| p.display().to_string())
            .collect::<Vec<_>>()
            .join("\n  ")
    ))
}

fn run_remove_chroma(
    input: &Path,
    out: &Path,
    key_color: &str,
    auto_key: &str,
    tolerance: u32,
    soft_matte: bool,
    transparent_threshold: f32,
    opaque_threshold: f32,
    edge_feather: f32,
    edge_contract: u32,
    despill: bool,
    force: bool,
    codex_home: Option<&Path>,
) -> Result<(), String> {
    if !input.is_file() {
        return Err(format!("input image not found: {}", input.display()));
    }
    let script = resolve_remove_chroma_script(codex_home)?;
    let mut cmd = Command::new("python3");
    cmd.arg(&script)
        .arg("--input")
        .arg(input)
        .arg("--out")
        .arg(out)
        .arg("--key-color")
        .arg(key_color)
        .arg("--auto-key")
        .arg(auto_key)
        .arg("--tolerance")
        .arg(tolerance.to_string())
        .arg("--transparent-threshold")
        .arg(transparent_threshold.to_string())
        .arg("--opaque-threshold")
        .arg(opaque_threshold.to_string())
        .arg("--edge-feather")
        .arg(edge_feather.to_string())
        .arg("--edge-contract")
        .arg(edge_contract.to_string());
    if soft_matte {
        cmd.arg("--soft-matte");
    }
    if despill {
        cmd.arg("--despill");
    }
    if force {
        cmd.arg("--force");
    }
    let status = cmd
        .status()
        .map_err(|e| format!("failed to run python3 {}: {e}", script.display()))?;
    if !status.success() {
        return Err(format!(
            "remove_chroma_key.py failed with status {status} (script {})",
            script.display()
        ));
    }
    Ok(())
}

struct BatchJob {
    index: usize,
    line_no: usize,
    out: PathBuf,
    body: Value,
}

/// Run batch jobs with a fixed worker pool. Results keep job order (index asc).
fn run_batch_parallel(
    jobs: &[BatchJob],
    base_url: &str,
    api_key: &str,
    timeout_secs: u64,
    output_format: OutputFormat,
    concurrency: usize,
) -> Result<Vec<Result<Vec<PathBuf>, String>>, String> {
    let workers = concurrency.min(jobs.len()).max(1);
    eprintln!(
        "batch: {} jobs, concurrency={workers}",
        jobs.len()
    );

    // Shared work queue: (result_slot, job)
    let queue: Arc<Mutex<Vec<(usize, BatchJob)>>> = Arc::new(Mutex::new(
        jobs.iter()
            .enumerate()
            .map(|(slot, j)| {
                (
                    slot,
                    BatchJob {
                        index: j.index,
                        line_no: j.line_no,
                        out: j.out.clone(),
                        body: j.body.clone(),
                    },
                )
            })
            .rev() // pop from end → process in original order preferentially
            .collect(),
    ));

    let results: Arc<Mutex<Vec<Option<Result<Vec<PathBuf>, String>>>>> =
        Arc::new(Mutex::new(vec![None; jobs.len()]));

    let base_url = base_url.to_string();
    let api_key = api_key.to_string();

    thread::scope(|scope| {
        for _ in 0..workers {
            let queue = Arc::clone(&queue);
            let results = Arc::clone(&results);
            let base_url = base_url.clone();
            let api_key = api_key.clone();
            scope.spawn(move || {
                loop {
                    let next = {
                        let mut q = queue.lock().expect("batch queue lock");
                        q.pop()
                    };
                    let Some((slot, job)) = next else {
                        break;
                    };
                    let outcome = (|| {
                        let resp = post_json(
                            &base_url,
                            "/images/generations",
                            &api_key,
                            &job.body,
                            timeout_secs,
                        )
                        .map_err(|e| {
                            format!("job {} (line {}): {e}", job.index, job.line_no)
                        })?;
                        save_response(&resp, &job.out, None, output_format, 1).map_err(|e| {
                            format!("job {} (line {}): {e}", job.index, job.line_no)
                        })
                    })();
                    let mut slots = results.lock().expect("batch results lock");
                    slots[slot] = Some(outcome);
                }
            });
        }
    });

    let slots = results.lock().expect("batch results lock");
    Ok(slots
        .iter()
        .map(|o| {
            o.clone().unwrap_or_else(|| {
                Err("internal: batch worker did not produce a result".into())
            })
        })
        .collect())
}

fn client(timeout_secs: u64) -> Result<reqwest::blocking::Client, String> {
    reqwest::blocking::Client::builder()
        .timeout(Duration::from_secs(timeout_secs))
        .build()
        .map_err(|e| format!("http client: {e}"))
}

fn auth_headers(api_key: &str) -> Result<HeaderMap, String> {
    let mut headers = HeaderMap::new();
    let value = HeaderValue::from_str(&format!("Bearer {api_key}"))
        .map_err(|e| format!("invalid api key header: {e}"))?;
    headers.insert(AUTHORIZATION, value);
    Ok(headers)
}

fn post_json(
    base_url: &str,
    path: &str,
    api_key: &str,
    body: &Value,
    timeout_secs: u64,
) -> Result<ImagesResponse, String> {
    let url = format!("{base_url}{path}");
    let client = client(timeout_secs)?;
    let resp = client
        .post(&url)
        .headers(auth_headers(api_key)?)
        .header(CONTENT_TYPE, "application/json")
        .json(body)
        .send()
        .map_err(|e| format!("request failed: {e}"))?;
    parse_images_response(resp)
}

fn post_edit_multipart(
    base_url: &str,
    api_key: &str,
    prompt: &str,
    images: &[PathBuf],
    mask: Option<&Path>,
    model: &str,
    size: &str,
    quality: &str,
    n: u8,
    output_format: OutputFormat,
    background: Option<Background>,
    input_fidelity: Option<InputFidelity>,
    timeout_secs: u64,
) -> Result<ImagesResponse, String> {
    let url = format!("{base_url}/images/edits");
    let client = client(timeout_secs)?;

    let mut form = multipart::Form::new()
        .text("prompt", prompt.to_string())
        .text("model", model.to_string())
        .text("size", size.to_string())
        .text("quality", quality.to_string())
        .text("n", n.to_string())
        .text("output_format", output_format.as_str().to_string())
        .text("response_format", "b64_json".to_string());

    if let Some(bg) = background {
        form = form.text("background", bg.as_str().to_string());
    }
    if let Some(f) = input_fidelity {
        form = form.text("input_fidelity", f.as_str().to_string());
    }

    for (i, img) in images.iter().enumerate() {
        let bytes = fs::read(img).map_err(|e| format!("read {}: {e}", img.display()))?;
        let filename = img
            .file_name()
            .and_then(|s| s.to_str())
            .unwrap_or("image.png")
            .to_string();
        let mime = guess_mime(&filename);
        let part = multipart::Part::bytes(bytes)
            .file_name(filename)
            .mime_str(mime)
            .map_err(|e| format!("multipart image: {e}"))?;
        let _ = i;
        // OpenAI accepts repeated "image" fields for multi-image edits.
        form = form.part("image", part);
    }

    if let Some(mask_path) = mask {
        let bytes = fs::read(mask_path).map_err(|e| format!("read mask: {e}"))?;
        let filename = mask_path
            .file_name()
            .and_then(|s| s.to_str())
            .unwrap_or("mask.png")
            .to_string();
        let part = multipart::Part::bytes(bytes)
            .file_name(filename)
            .mime_str("image/png")
            .map_err(|e| format!("multipart mask: {e}"))?;
        form = form.part("mask", part);
    }

    let resp = client
        .post(&url)
        .headers(auth_headers(api_key)?)
        .multipart(form)
        .send()
        .map_err(|e| format!("request failed: {e}"))?;
    parse_images_response(resp)
}

fn guess_mime(filename: &str) -> &'static str {
    let lower = filename.to_ascii_lowercase();
    if lower.ends_with(".jpg") || lower.ends_with(".jpeg") {
        "image/jpeg"
    } else if lower.ends_with(".webp") {
        "image/webp"
    } else {
        "image/png"
    }
}

fn parse_images_response(resp: reqwest::blocking::Response) -> Result<ImagesResponse, String> {
    let status = resp.status();
    let text = resp
        .text()
        .map_err(|e| format!("failed to read response body: {e}"))?;
    if !status.is_success() {
        return Err(format!("HTTP {status}: {text}"));
    }
    serde_json::from_str::<ImagesResponse>(&text).map_err(|e| {
        format!("failed to parse images response: {e}; body={}", truncate(&text, 800))
    })
}

fn truncate(s: &str, max: usize) -> String {
    if s.len() <= max {
        s.to_string()
    } else {
        format!("{}…", &s[..max])
    }
}

fn save_response(
    resp: &ImagesResponse,
    out: &Path,
    out_dir: Option<&Path>,
    output_format: OutputFormat,
    expected_n: usize,
) -> Result<Vec<PathBuf>, String> {
    if resp.data.is_empty() {
        return Err("upstream returned no images".into());
    }
    let count = resp.data.len().max(1);
    let paths = build_output_paths(out, out_dir, output_format, count.max(expected_n.min(count)))?;

    let mut written = Vec::new();
    for (i, item) in resp.data.iter().enumerate() {
        let path = paths
            .get(i)
            .cloned()
            .unwrap_or_else(|| paths[0].with_extension(format!("{i}.{}", output_format.extension())));
        if let Some(parent) = path.parent() {
            if !parent.as_os_str().is_empty() {
                fs::create_dir_all(parent)
                    .map_err(|e| format!("mkdir {}: {e}", parent.display()))?;
            }
        }
        if let Some(b64) = &item.b64_json {
            let bytes = base64::engine::general_purpose::STANDARD
                .decode(b64.trim())
                .map_err(|e| format!("base64 decode failed: {e}"))?;
            fs::write(&path, bytes).map_err(|e| format!("write {}: {e}", path.display()))?;
            written.push(path);
        } else if let Some(url) = &item.url {
            // Optional: download URL results.
            let client = client(120)?;
            let bytes = client
                .get(url)
                .send()
                .and_then(|r| r.error_for_status())
                .and_then(|r| r.bytes())
                .map_err(|e| format!("download image url failed: {e}"))?;
            fs::write(&path, &bytes).map_err(|e| format!("write {}: {e}", path.display()))?;
            written.push(path);
        } else {
            return Err(format!("image[{i}] has neither b64_json nor url"));
        }
    }
    Ok(written)
}

fn build_output_paths(
    out: &Path,
    out_dir: Option<&Path>,
    output_format: OutputFormat,
    count: usize,
) -> Result<Vec<PathBuf>, String> {
    let ext = output_format.extension();
    if let Some(dir) = out_dir {
        fs::create_dir_all(dir).map_err(|e| format!("mkdir {}: {e}", dir.display()))?;
        return Ok((1..=count)
            .map(|i| dir.join(format!("image_{i}.{ext}")))
            .collect());
    }

    let mut path = out.to_path_buf();
    if path.extension().is_none() {
        path.set_extension(ext);
    }
    if let Some(parent) = path.parent() {
        if !parent.as_os_str().is_empty() {
            fs::create_dir_all(parent).map_err(|e| format!("mkdir {}: {e}", parent.display()))?;
        }
    }
    if count == 1 {
        return Ok(vec![path]);
    }
    let stem = path
        .file_stem()
        .and_then(|s| s.to_str())
        .unwrap_or("output")
        .to_string();
    let parent = path.parent().unwrap_or_else(|| Path::new("."));
    Ok((1..=count)
        .map(|i| parent.join(format!("{stem}-{i}.{ext}")))
        .collect())
}

