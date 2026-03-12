import requests
import json

class Client:
    """
    Client for the MyGamesAnywhere Server REST API.
    """
    def __init__(self, base_url="http://localhost:8080"):
        self.base_url = base_url.rstrip("/")

    def get_health(self):
        """Check server health."""
        resp = requests.get(f"{self.base_url}/health")
        resp.raise_for_status()
        return resp.text

    def list_games(self):
        """Retrieve all discovered games."""
        resp = requests.get(f"{self.base_url}/api/games")
        resp.raise_for_status()
        return resp.json()

    def get_game(self, game_id):
        """Retrieve details for a specific game."""
        resp = requests.get(f"{self.base_url}/api/games/{game_id}")
        resp.raise_for_status()
        return resp.json()

    def trigger_scan(self):
        """Trigger a new game discovery scan across all source plugins."""
        resp = requests.post(f"{self.base_url}/api/scan")
        resp.raise_for_status()
        return resp.json()

    def set_config(self, key, value):
        """Set a global server configuration key (e.g. Google Drive credentials)."""
        resp = requests.post(f"{self.base_url}/api/config/{key}", json={"value": value})
        resp.raise_for_status()
        return resp.status_code == 200

    def list_integrations(self):
        """List all configured integrations."""
        resp = requests.get(f"{self.base_url}/api/integrations")
        resp.raise_for_status()
        return resp.json()

    def get_integration_status(self):
        """Check the status of all configured integrations."""
        resp = requests.get(f"{self.base_url}/api/integrations/status")
        resp.raise_for_status()
        return resp.json()

    def create_integration(self, plugin_id, label, config_dict):
        """
        Connect a new integration (e.g. a specific Google Drive folder).
        config_dict: A dictionary matching the plugin's config schema.
        """
        data = {
            "plugin_id": plugin_id,
            "label": label,
            "config": config_dict
        }
        resp = requests.post(f"{self.base_url}/api/integrations", json=data)
        resp.raise_for_status()
        return resp.json()
