#!/usr/bin/env python3
"""
Test Power Plant API Server

Simulates the power plant API with configurable generation range.
Consumption is set via POST endpoint from PowerHive automation.
Uses random walk pattern to fluctuate generation realistically within bounds.
"""

import argparse
import json
import logging
import random
import threading
import time
from datetime import datetime, timezone
from flask import Flask, request, jsonify

app = Flask(__name__)

# Global state for power readings
class PowerState:
    def __init__(self, gen_min, gen_max, step):
        self.gen_min = gen_min
        self.gen_max = gen_max
        self.step = step

        # Initialize at random starting point within range
        self.generation_mw = random.uniform(gen_min, gen_max)

        # Consumption is set via POST endpoint, default to 0
        self.consumption_mw = 0.0

        # Thread safety
        self.lock = threading.Lock()

    def update(self):
        """Update generation using random walk pattern"""
        with self.lock:
            # Random walk for generation only
            delta_gen = random.uniform(-self.step, self.step)
            self.generation_mw += delta_gen
            self.generation_mw = max(self.gen_min, min(self.gen_max, self.generation_mw))

            logging.info(
                f"Updated: Generation={self.generation_mw:.2f} MW, "
                f"Consumption={self.consumption_mw:.2f} MW, "
                f"Available={self.generation_mw - self.consumption_mw:.2f} MW"
            )

    def set_consumption(self, consumption_mw):
        """Set consumption value from external source"""
        with self.lock:
            self.consumption_mw = consumption_mw
            logging.info(f"Consumption set to {self.consumption_mw:.2f} MW")

    def get_reading(self):
        """Get current reading with realistic splitting"""
        with self.lock:
            now = datetime.now(timezone.utc).isoformat()

            # Split generation between two sources (48-52% each, varying slightly)
            gen_split = random.uniform(0.48, 0.52)
            generoso_mw = self.generation_mw * gen_split
            nogueira_mw = self.generation_mw * (1 - gen_split)

            # Split consumption between two containers (45-55% each, varying slightly)
            cons_split = random.uniform(0.45, 0.55)
            container_eles_mw = self.consumption_mw * cons_split
            container_mazp_mw = self.consumption_mw * (1 - cons_split)

            return {
                "reading": {
                    "collection_timestamp": now,
                    "consumption": {
                        "container_eles": {
                            "source_timestamp": now,
                            "status": "success",
                            "value_mw": container_eles_mw
                        },
                        "container_mazp": {
                            "source_timestamp": now,
                            "status": "success",
                            "value_mw": container_mazp_mw
                        }
                    },
                    "generation": {
                        "generoso": {
                            "source_timestamp": now,
                            "status": "success",
                            "value_mw": generoso_mw
                        },
                        "nogueira": {
                            "source_timestamp": now,
                            "status": "success",
                            "value_mw": nogueira_mw
                        }
                    },
                    "id": int(time.time()),  # Use timestamp as incrementing ID
                    "plant_id": "complexo-paranhos",
                    "totals": {
                        "consumption_mw": self.consumption_mw,
                        "exported_mw": self.generation_mw - self.consumption_mw,
                        "generation_mw": self.generation_mw
                    },
                    "trust": {
                        "confidence_score": 1.0,
                        "status": "trusted",
                        "summary": "Test data - all checks passed"
                    }
                }
            }

# Global state instance
state = None
auth_token = None

@app.route('/data/latest', methods=['GET'])
def get_latest():
    """Endpoint matching the real power plant API"""
    # Check authentication
    auth_header = request.headers.get('Authorization', '')
    if not auth_header.startswith('Bearer '):
        return jsonify({"error": "Missing or invalid authorization header"}), 401

    token = auth_header.replace('Bearer ', '')
    if token != auth_token:
        return jsonify({"error": "Invalid API token"}), 401

    # Check plant_id parameter
    plant_id = request.args.get('plant_id')
    if plant_id != 'complexo-paranhos':
        return jsonify({"error": "Invalid plant_id"}), 400

    # Return current reading
    reading = state.get_reading()
    return jsonify(reading)

@app.route('/data/consumption', methods=['POST'])
def set_consumption():
    """Endpoint to receive expected consumption from PowerHive"""
    # No authentication required for test server
    try:
        data = request.get_json()
        if not data or 'expected_consumption_mw' not in data:
            return jsonify({"error": "Missing expected_consumption_mw field"}), 400

        consumption_mw = float(data['expected_consumption_mw'])
        if consumption_mw < 0:
            return jsonify({"error": "Consumption must be non-negative"}), 400

        state.set_consumption(consumption_mw)
        return jsonify({
            "status": "ok",
            "consumption_mw": round(consumption_mw, 2)
        })
    except (ValueError, TypeError) as e:
        return jsonify({"error": f"Invalid consumption value: {str(e)}"}), 400

@app.route('/health', methods=['GET'])
def health():
    """Health check endpoint (no auth required)"""
    return jsonify({
        "status": "ok",
        "generation_range": [state.gen_min, state.gen_max],
        "current_generation": round(state.generation_mw, 2),
        "current_consumption": round(state.consumption_mw, 2),
        "consumption_source": "external"  # Indicates consumption is set via POST
    })

def update_loop(interval):
    """Background thread to update values periodically"""
    while True:
        time.sleep(interval)
        state.update()

def main():
    global state, auth_token

    parser = argparse.ArgumentParser(
        description='Test Power Plant API Server',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog='''
Examples:
  # Small scale test (50-150 kW generation)
  python test-plant-server.py --gen-min 0.05 --gen-max 0.15

  # Medium scale test (500-1500 kW generation)
  python test-plant-server.py --gen-min 0.5 --gen-max 1.5

  # With custom fluctuation rate (faster changes)
  python test-plant-server.py --gen-min 0.1 --gen-max 0.5 --step 0.005 --interval 15

Note: All power values are in MW (megawatts). 1 MW = 1000 kW.
Consumption is set via POST /data/consumption endpoint by PowerHive automation.
        '''
    )

    parser.add_argument('--gen-min', type=float, required=True,
                        help='Minimum generation in MW')
    parser.add_argument('--gen-max', type=float, required=True,
                        help='Maximum generation in MW')
    parser.add_argument('--step', type=float, default=0.002,
                        help='Maximum change per update (MW, default: 0.002)')
    parser.add_argument('--interval', type=int, default=30,
                        help='Update interval in seconds (default: 30)')
    parser.add_argument('--port', type=int, default=8090,
                        help='HTTP server port (default: 8090)')
    parser.add_argument('--token', type=str,
                        default='7ab39eed3b05b7e4efc17285bb416304256979967ec8d6ad2b2ef3bc10c0f5ed',
                        help='Bearer token for authentication (default: matches config.json)')
    parser.add_argument('--debug', action='store_true',
                        help='Enable debug logging')

    args = parser.parse_args()

    # Validate ranges
    if args.gen_min >= args.gen_max:
        parser.error('gen-min must be less than gen-max')
    if args.step <= 0:
        parser.error('step must be positive')

    # Setup logging
    log_level = logging.DEBUG if args.debug else logging.INFO
    logging.basicConfig(
        level=log_level,
        format='%(asctime)s - %(levelname)s - %(message)s'
    )

    # Initialize state
    state = PowerState(
        args.gen_min, args.gen_max,
        args.step
    )
    auth_token = args.token

    logging.info('='*70)
    logging.info('Test Power Plant API Server Starting')
    logging.info('='*70)
    logging.info(f'Generation range: {args.gen_min} - {args.gen_max} MW')
    logging.info(f'Consumption: Set via POST /data/consumption (default: 0 MW)')
    logging.info(f'Random walk step: ±{args.step} MW')
    logging.info(f'Update interval: {args.interval} seconds')
    logging.info(f'Server port: {args.port}')
    logging.info(f'Auth token: {auth_token[:20]}...')
    logging.info('='*70)
    logging.info(f'Initial generation: {state.generation_mw:.2f} MW')
    logging.info(f'Initial consumption: {state.consumption_mw:.2f} MW (awaiting POST)')
    logging.info(f'Initial available: {state.generation_mw - state.consumption_mw:.2f} MW')
    logging.info('='*70)
    logging.info(f'Data endpoint: http://localhost:{args.port}/data/latest?plant_id=complexo-paranhos')
    logging.info(f'Consumption POST: http://localhost:{args.port}/data/consumption')
    logging.info(f'Health check: http://localhost:{args.port}/health')
    logging.info('='*70)

    # Start background update thread
    update_thread = threading.Thread(target=update_loop, args=(args.interval,), daemon=True)
    update_thread.start()

    # Start Flask server
    app.run(host='0.0.0.0', port=args.port, debug=False, threaded=True)

if __name__ == '__main__':
    main()
