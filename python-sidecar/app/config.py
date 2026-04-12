"""Application configuration loaded from environment variables."""

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """Sidecar configuration — all values can be overridden via env vars."""

    ollama_base_url: str = "http://ollama:11434"
    ollama_default_model: str = "llama3"
    openai_api_key: str = ""
    openai_base_url: str = "https://api.openai.com/v1"
    openai_default_model: str = "gpt-4o-mini"
    sidecar_secret: str = "change-me-sidecar-secret"
    log_level: str = "info"

    model_config = {"env_prefix": "", "case_sensitive": False}


settings = Settings()
