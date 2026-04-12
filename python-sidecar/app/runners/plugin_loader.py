"""Plugin loader — dynamically loads and executes user-defined LangGraph workflows."""

from __future__ import annotations

import importlib
import logging
import sys
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

PLUGINS_DIR = Path("/app/plugins")


def discover_plugins() -> list[dict]:
    """Discover all available plugins in the plugins directory."""
    plugins = []
    if not PLUGINS_DIR.exists():
        return plugins

    for plugin_dir in PLUGINS_DIR.iterdir():
        if not plugin_dir.is_dir():
            continue
        config_file = plugin_dir / "config.yaml"
        workflow_file = plugin_dir / "workflow.py"

        plugin_info: dict[str, Any] = {
            "name": plugin_dir.name,
            "has_config": config_file.exists(),
            "has_workflow": workflow_file.exists(),
            "path": str(plugin_dir),
        }

        if config_file.exists():
            try:
                import yaml  # noqa: F811

                with open(config_file) as f:
                    plugin_info["config"] = yaml.safe_load(f)
            except Exception:
                plugin_info["config"] = {}

        plugins.append(plugin_info)

    return plugins


def load_plugin_graph(plugin_name: str) -> Any:
    """Load a plugin's LangGraph workflow by importing its workflow module.

    The plugin must have a `build_graph()` function that returns a StateGraph.
    """
    plugin_path = PLUGINS_DIR / plugin_name
    workflow_file = plugin_path / "workflow.py"

    if not workflow_file.exists():
        raise FileNotFoundError(f"Plugin '{plugin_name}' has no workflow.py")

    # Add plugin directory to sys.path temporarily.
    plugin_dir_str = str(plugin_path)
    if plugin_dir_str not in sys.path:
        sys.path.insert(0, plugin_dir_str)

    try:
        # Use importlib to load the module.
        module_name = f"plugins.{plugin_name}.workflow"
        if module_name in sys.modules:
            # Force reload for hot-reloading.
            module = importlib.reload(sys.modules[module_name])
        else:
            spec = importlib.util.spec_from_file_location(module_name, str(workflow_file))
            if spec is None or spec.loader is None:
                raise ImportError(f"Cannot load plugin module: {workflow_file}")
            module = importlib.util.module_from_spec(spec)
            sys.modules[module_name] = module
            spec.loader.exec_module(module)

        if not hasattr(module, "build_graph"):
            raise AttributeError(f"Plugin '{plugin_name}' missing build_graph() function")

        return module.build_graph()

    except Exception:
        logger.exception(f"Failed to load plugin '{plugin_name}'")
        raise
    finally:
        if plugin_dir_str in sys.path:
            sys.path.remove(plugin_dir_str)


def validate_permissions(permissions: list[str]) -> dict[str, bool]:
    """Validate and return effective permissions for a plugin execution."""
    allowed = {"llm_access", "http_access", "no_shell"}
    result = {}
    for perm in permissions:
        result[perm] = perm in allowed
    return result
