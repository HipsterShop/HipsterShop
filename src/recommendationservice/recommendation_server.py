
import os
import random

import requests
from flask import Flask, request as flask_request, jsonify

from logger import getJSONLogger
logger = getJSONLogger('recommendationservice-server')

app = Flask(__name__)

catalog_addr = ""

def initStackdriverProfiling():
  project_id = None
  try:
    project_id = os.environ["GCP_PROJECT_ID"]
  except KeyError:
    pass
  return

@app.route('/recommendations', methods=['POST'])
def list_recommendations():
  data = flask_request.get_json(silent=True) or {}
  user_id = data.get('userId', '')
  product_ids = data.get('productIds', [])

  max_responses = 5
  # Call product catalog service via REST
  try:
    resp = requests.get(f"http://{catalog_addr}/products", timeout=5)
    resp.raise_for_status()
    cat_response = resp.json()
    all_product_ids = [x['id'] for x in cat_response.get('products', [])]
  except Exception as e:
    logger.error(f"Failed to get products from catalog: {e}")
    return jsonify({"productIds": []}), 500

  filtered_products = list(set(all_product_ids) - set(product_ids))
  num_products = len(filtered_products)
  num_return = min(max_responses, num_products)
  if num_products > 0:
    indices = random.sample(range(num_products), num_return)
    prod_list = [filtered_products[i] for i in indices]
  else:
    prod_list = []

  logger.info("[Recv ListRecommendations] product_ids={}".format(prod_list))
  return jsonify({"productIds": prod_list})

@app.route('/_healthz', methods=['GET'])
def health_check():
    return 'ok'


if __name__ == "__main__":
    logger.info("initializing recommendationservice")

    try:
      if "DISABLE_PROFILER" in os.environ:
        raise KeyError()
      else:
        logger.info("Profiler enabled.")
        initStackdriverProfiling()
    except KeyError:
        logger.info("Profiler disabled.")

    port = os.environ.get('PORT', "8080")
    catalog_addr = os.environ.get('PRODUCT_CATALOG_SERVICE_ADDR', '')
    if catalog_addr == "":
        raise Exception('PRODUCT_CATALOG_SERVICE_ADDR environment variable not set')
    logger.info("product catalog address: " + catalog_addr)

    logger.info("listening on port: " + port)
    app.run(host='0.0.0.0', port=int(port))
