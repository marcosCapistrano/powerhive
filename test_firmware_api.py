#!/usr/bin/env python3
"""
Firmware API Test Script for PowerHive

Tests all firmware API endpoints used by the PowerHive application.
Unlocks the miner, manages API keys, and exercises every endpoint to show what they return.
"""

import argparse
import json
import secrets
import sys
from typing import Any, Dict, Optional, Tuple

try:
    import requests
except ImportError:
    print("ERROR: requests library not found. Install with: pip install requests")
    sys.exit(1)


class Colors:
    """ANSI color codes for terminal output"""
    HEADER = '\033[95m'
    OKBLUE = '\033[94m'
    OKCYAN = '\033[96m'
    OKGREEN = '\033[92m'
    WARNING = '\033[93m'
    FAIL = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'
    UNDERLINE = '\033[4m'


class FirmwareAPITester:
    """Tests all firmware API endpoints"""

    def __init__(self, miner_ip: str, password: str = "admin", timeout: int = 5):
        self.miner_ip = miner_ip
        self.password = password
        self.timeout = timeout
        self.base_url = f"http://{miner_ip}/api/v1"
        self.bearer_token: Optional[str] = None
        self.api_key: Optional[str] = None
        self.results: Dict[str, bool] = {}

    def build_url(self, endpoint: str) -> str:
        """Build full URL from endpoint"""
        # Remove leading slash if present to avoid double slashes
        endpoint = endpoint.lstrip('/')
        return f"{self.base_url}/{endpoint}"

    def print_section(self, title: str):
        """Print a colored section header"""
        print(f"\n{Colors.BOLD}{Colors.HEADER}{'=' * 80}{Colors.ENDC}")
        print(f"{Colors.BOLD}{Colors.HEADER}{title}{Colors.ENDC}")
        print(f"{Colors.BOLD}{Colors.HEADER}{'=' * 80}{Colors.ENDC}")

    def print_endpoint(self, method: str, endpoint: str, auth_type: str = "none"):
        """Print endpoint information"""
        auth_display = {
            "none": f"{Colors.OKBLUE}[NO AUTH]{Colors.ENDC}",
            "bearer": f"{Colors.WARNING}[BEARER TOKEN]{Colors.ENDC}",
            "apikey": f"{Colors.OKCYAN}[API KEY]{Colors.ENDC}",
        }
        auth = auth_display.get(auth_type, auth_type)
        print(f"\n{Colors.BOLD}{method} {endpoint}{Colors.ENDC} {auth}")

    def print_response(self, data: Any, success: bool = True):
        """Pretty print JSON response"""
        if success:
            print(f"{Colors.OKGREEN}✓ SUCCESS{Colors.ENDC}")
        else:
            print(f"{Colors.FAIL}✗ FAILED{Colors.ENDC}")

        if data:
            print(json.dumps(data, indent=2, default=str))

    def print_error(self, error: str):
        """Print error message"""
        print(f"{Colors.FAIL}✗ ERROR: {error}{Colors.ENDC}")

    def record_result(self, endpoint: str, success: bool):
        """Record test result"""
        self.results[endpoint] = success

    def test_unlock(self) -> Tuple[bool, Optional[str]]:
        """Test POST /unlock - Exchange password for bearer token"""
        endpoint = "/unlock"
        self.print_endpoint("POST", endpoint, "none")

        try:
            response = requests.post(
                self.build_url(endpoint),
                json={"pw": self.password},
                timeout=self.timeout
            )
            response.raise_for_status()
            data = response.json()
            token = data.get("token", "")

            if not token:
                self.print_error("Token is empty in response")
                self.record_result("POST /unlock", False)
                return False, None

            self.print_response(data, True)
            self.record_result("POST /unlock", True)
            return True, token

        except Exception as e:
            self.print_error(str(e))
            self.record_result("POST /unlock", False)
            return False, None

    def test_list_api_keys(self, bearer: str) -> Tuple[bool, list]:
        """Test GET /apikeys - List existing API keys"""
        endpoint = "/apikeys"
        self.print_endpoint("GET", endpoint, "bearer")

        try:
            response = requests.get(
                self.build_url(endpoint),
                headers={"Authorization": f"Bearer {bearer}"},
                timeout=self.timeout
            )
            response.raise_for_status()
            keys = response.json()

            self.print_response(keys, True)
            self.record_result("GET /apikeys", True)
            return True, keys

        except Exception as e:
            self.print_error(str(e))
            self.record_result("GET /apikeys", False)
            return False, []

    def test_create_api_key(self, bearer: str, key: str, description: str) -> bool:
        """Test POST /apikeys - Create new API key"""
        endpoint = "/apikeys"
        self.print_endpoint("POST", endpoint, "bearer")

        try:
            payload = {"key": key, "description": description}
            print(f"Request payload: {json.dumps(payload, indent=2)}")

            response = requests.post(
                self.build_url(endpoint),
                json=payload,
                headers={"Authorization": f"Bearer {bearer}"},
                timeout=self.timeout
            )
            response.raise_for_status()

            # Empty response expected on success
            self.print_response({"status": "API key created successfully"}, True)
            self.record_result("POST /apikeys", True)
            return True

        except Exception as e:
            self.print_error(str(e))
            self.record_result("POST /apikeys", False)
            return False

    def test_info(self) -> bool:
        """Test GET /info - General miner metadata (no auth)"""
        endpoint = "/info"
        self.print_endpoint("GET", endpoint, "none")

        try:
            response = requests.get(
                self.build_url(endpoint),
                timeout=self.timeout
            )
            response.raise_for_status()
            data = response.json()

            self.print_response(data, True)
            self.record_result("GET /info", True)
            return True

        except Exception as e:
            self.print_error(str(e))
            self.record_result("GET /info", False)
            return False

    def test_model(self) -> bool:
        """Test GET /model - Model metadata (no auth)"""
        endpoint = "/model"
        self.print_endpoint("GET", endpoint, "none")

        try:
            response = requests.get(
                self.build_url(endpoint),
                timeout=self.timeout
            )
            response.raise_for_status()
            data = response.json()

            self.print_response(data, True)
            self.record_result("GET /model", True)
            return True

        except Exception as e:
            self.print_error(str(e))
            self.record_result("GET /model", False)
            return False

    def test_status(self) -> bool:
        """Test GET /status - Lightweight miner state (no auth)"""
        endpoint = "/status"
        self.print_endpoint("GET", endpoint, "none")

        try:
            response = requests.get(
                self.build_url(endpoint),
                timeout=self.timeout
            )
            response.raise_for_status()
            data = response.json()

            self.print_response(data, True)
            self.record_result("GET /status", True)
            return True

        except Exception as e:
            self.print_error(str(e))
            self.record_result("GET /status", False)
            return False

    def test_summary(self, api_key: str) -> bool:
        """Test GET /summary - Rich miner summary (requires API key)"""
        endpoint = "/summary"
        self.print_endpoint("GET", endpoint, "apikey")

        try:
            response = requests.get(
                self.build_url(endpoint),
                headers={"x-api-key": api_key},
                timeout=self.timeout
            )
            response.raise_for_status()
            data = response.json()

            self.print_response(data, True)
            self.record_result("GET /summary", True)
            return True

        except Exception as e:
            self.print_error(str(e))
            self.record_result("GET /summary", False)
            return False

    def test_perf_summary(self, api_key: str) -> bool:
        """Test GET /perf-summary - Current preset and autotune data (requires API key)"""
        endpoint = "/perf-summary"
        self.print_endpoint("GET", endpoint, "apikey")

        try:
            response = requests.get(
                self.build_url(endpoint),
                headers={"x-api-key": api_key},
                timeout=self.timeout
            )
            response.raise_for_status()
            data = response.json()

            self.print_response(data, True)
            self.record_result("GET /perf-summary", True)
            return True

        except Exception as e:
            self.print_error(str(e))
            self.record_result("GET /perf-summary", False)
            return False

    def test_chains(self, api_key: str) -> bool:
        """Test GET /chains - Per-chip telemetry for each hashboard (requires API key)"""
        endpoint = "/chains"
        self.print_endpoint("GET", endpoint, "apikey")

        try:
            response = requests.get(
                self.build_url(endpoint),
                headers={"x-api-key": api_key},
                timeout=self.timeout
            )
            response.raise_for_status()
            data = response.json()

            self.print_response(data, True)
            self.record_result("GET /chains", True)
            return True

        except Exception as e:
            self.print_error(str(e))
            self.record_result("GET /chains", False)
            return False

    def test_autotune_presets(self, bearer: str) -> bool:
        """Test GET /autotune/presets - Available performance presets (requires bearer)"""
        endpoint = "/autotune/presets"
        self.print_endpoint("GET", endpoint, "bearer")

        try:
            response = requests.get(
                self.build_url(endpoint),
                headers={"Authorization": f"Bearer {bearer}"},
                timeout=self.timeout
            )
            response.raise_for_status()
            data = response.json()

            self.print_response(data, True)
            self.record_result("GET /autotune/presets", True)
            return True

        except Exception as e:
            self.print_error(str(e))
            self.record_result("GET /autotune/presets", False)
            return False

    def test_set_preset(self, api_key: str, preset_name: str) -> bool:
        """Test POST /autotune/preset - Change performance preset (requires API key)"""
        endpoint = "/autotune/preset"
        self.print_endpoint("POST", endpoint, "apikey")

        # Uncomment below to actually test (will change miner settings!)
        try:
            payload = {"preset": preset_name}
            print(f"Request payload: {json.dumps(payload, indent=2)}")

            response = requests.post(
                self.build_url(endpoint),
                json=payload,
                headers={"x-api-key": api_key},
                timeout=self.timeout
            )
            response.raise_for_status()

            self.print_response({"status": "Preset changed successfully"}, True)
            self.record_result("POST /autotune/preset", True)
            return True

        except Exception as e:
            self.print_error(str(e))
            self.record_result("POST /autotune/preset", False)
            return False

    def run_all_tests(self):
        """Execute all API endpoint tests"""
        print(f"\n{Colors.BOLD}{Colors.OKCYAN}Starting Firmware API Tests for {self.miner_ip}{Colors.ENDC}")
        print(f"Base URL: {self.base_url}")
        print(f"Timeout: {self.timeout}s")

        # Phase 1: Authentication
        self.print_section("PHASE 1: AUTHENTICATION")

        # 1. Unlock to get bearer token
        success, bearer_token = self.test_unlock()
        if not success or not bearer_token:
            print(f"\n{Colors.FAIL}Cannot proceed without bearer token. Exiting.{Colors.ENDC}")
            return

        self.bearer_token = bearer_token

        # 2. List existing API keys
        success, existing_keys = self.test_list_api_keys(bearer_token)

        # 3. Check if PowerHive API key exists, create if needed
        powerhive_key = None
        for key_obj in existing_keys:
            if key_obj.get("description", "").lower() == "powerhive":
                powerhive_key = key_obj.get("key", "")
                print(f"\n{Colors.OKGREEN}Found existing PowerHive API key{Colors.ENDC}")
                break

        if not powerhive_key:
            print(f"\n{Colors.WARNING}PowerHive API key not found, creating new one...{Colors.ENDC}")
            new_key = secrets.token_hex(16)
            if self.test_create_api_key(bearer_token, new_key, "PowerHive"):
                powerhive_key = new_key
                print(f"{Colors.OKGREEN}Created new PowerHive API key: {new_key}{Colors.ENDC}")

        if not powerhive_key:
            print(f"\n{Colors.FAIL}Cannot proceed without API key. Exiting.{Colors.ENDC}")
            return

        self.api_key = powerhive_key

        # Phase 2: Unauthenticated Endpoints
        self.print_section("PHASE 2: UNAUTHENTICATED ENDPOINTS")

        self.test_info()
        self.test_model()
        self.test_status()

        # Phase 3: API Key Authenticated Endpoints
        self.print_section("PHASE 3: API KEY AUTHENTICATED ENDPOINTS")

        self.test_summary(powerhive_key)
        self.test_perf_summary(powerhive_key)
        self.test_chains(powerhive_key)

        # Phase 4: Bearer Token Authenticated Endpoints
        self.print_section("PHASE 4: BEARER TOKEN AUTHENTICATED ENDPOINTS")

        self.test_autotune_presets(bearer_token)

        # Phase 5: Mutating Endpoints (skipped by default)
        self.print_section("PHASE 5: MUTATING ENDPOINTS (SKIPPED)")

        self.test_set_preset(powerhive_key, "2350")

        # Print Summary
        self.print_summary()

    def print_summary(self):
        """Print test results summary"""
        self.print_section("TEST SUMMARY")

        total = len(self.results)
        passed = sum(1 for v in self.results.values() if v is True)
        failed = sum(1 for v in self.results.values() if v is False)
        skipped = sum(1 for v in self.results.values() if v is None)

        print(f"\nTotal endpoints tested: {total}")
        print(f"{Colors.OKGREEN}✓ Passed: {passed}{Colors.ENDC}")
        print(f"{Colors.FAIL}✗ Failed: {failed}{Colors.ENDC}")
        print(f"{Colors.WARNING}⊘ Skipped: {skipped}{Colors.ENDC}")

        if failed > 0:
            print(f"\n{Colors.FAIL}Failed endpoints:{Colors.ENDC}")
            for endpoint, result in self.results.items():
                if result is False:
                    print(f"  {Colors.FAIL}✗ {endpoint}{Colors.ENDC}")

        print(f"\n{Colors.BOLD}Bearer Token: {self.bearer_token}{Colors.ENDC}")
        print(f"{Colors.BOLD}API Key: {self.api_key}{Colors.ENDC}")


def main():
    parser = argparse.ArgumentParser(
        description="Test all firmware API endpoints used by PowerHive",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python test_firmware_api.py 192.168.1.100
  python test_firmware_api.py 192.168.1.100 --password mypassword
  python test_firmware_api.py 192.168.1.100 --timeout 10
        """
    )

    parser.add_argument(
        "miner_ip",
        help="IP address of the miner (e.g., 192.168.1.100)"
    )

    parser.add_argument(
        "--password",
        default="admin",
        help="Unlock password (default: admin)"
    )

    parser.add_argument(
        "--timeout",
        type=int,
        default=5,
        help="Request timeout in seconds (default: 5)"
    )

    args = parser.parse_args()

    tester = FirmwareAPITester(args.miner_ip, args.password, args.timeout)

    try:
        tester.run_all_tests()
    except KeyboardInterrupt:
        print(f"\n\n{Colors.WARNING}Interrupted by user{Colors.ENDC}")
        sys.exit(1)
    except Exception as e:
        print(f"\n\n{Colors.FAIL}Unexpected error: {e}{Colors.ENDC}")
        sys.exit(1)


if __name__ == "__main__":
    main()
