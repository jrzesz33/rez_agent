"""
Course Configuration Loader
Loads golf course information from the courseInfo.yaml file.
"""
import logging
import os
from pathlib import Path
from typing import Dict, Any

import yaml

logger = logging.getLogger()


def load_course_config() -> Dict[str, Any]:
    """Load course configuration from courseInfo.yaml"""
    try:
        # Find the courseInfo.yaml file
        config_path = Path("/var/task/pkg/courses/courseInfo.yaml")

        # If running locally, use relative path
        if not config_path.exists():
            config_path = Path(__file__).parent.parent.parent / "pkg" / "courses" / "courseInfo.yaml"

        if not config_path.exists():
            logger.error(f"Course config not found at: {config_path}")
            return {"courses": []}

        # Load YAML
        with open(config_path, "r") as f:
            config = yaml.safe_load(f)

        logger.info(f"Loaded course configuration with {len(config.get('courses', []))} courses")
        return config

    except Exception as e:
        logger.error(f"Error loading course config: {e}", exc_info=True)
        return {"courses": []}


def get_course_by_name(course_name: str) -> Dict[str, Any]:
    """Get course configuration by name"""
    config = load_course_config()

    for course in config.get("courses", []):
        if course_name.lower() in course.get("name", "").lower():
            return course

    return {}
