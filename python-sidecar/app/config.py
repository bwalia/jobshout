"""Application configuration loaded from environment variables."""

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """Sidecar configuration — all values can be overridden via env vars."""

    ollama_base_url: str = "http://ollama:11434"
    ollama_default_model: str = "llama3"
    # Empty means "no gateway": requests to Ollama go out unsigned. Set to the
    # shared workstation-gateway secret to sign each request (see app/llm.py).
    ollama_jwt_secret: str = ""
    openai_api_key: str = ""
    openai_base_url: str = "https://api.openai.com/v1"
    openai_default_model: str = "gpt-4o-mini"
    sidecar_secret: str = "change-me-sidecar-secret"
    log_level: str = "info"
    # Langfuse LLM observability. Tracing is on only when both keys are set;
    # left empty, every code path behaves exactly as before.
    langfuse_host: str = ""
    langfuse_public_key: str = ""
    langfuse_secret_key: str = ""

    model_config = {"env_prefix": "", "case_sensitive": False}


settings = Settings()
