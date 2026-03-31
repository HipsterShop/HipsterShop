
import os
import sys
import time
import traceback
import json
from datetime import datetime
from flask import Flask, request, jsonify
from jinja2 import Environment, FileSystemLoader, select_autoescape, TemplateError
from google.auth.exceptions import DefaultCredentialsError
from pymongo import MongoClient

from logger import getJSONLogger
logger = getJSONLogger('emailservice-server')

env = Environment(
    loader=FileSystemLoader('templates'),
    autoescape=select_autoescape(['html', 'xml'])
)
template = env.get_template('confirmation.html')

mongo_client = None
email_events_collection = None


def init_mongo_store():
  global mongo_client, email_events_collection
  mongo_uri = os.environ.get('EMAIL_MONGO_URI') or os.environ.get('MONGO_URI')
  if not mongo_uri:
    logger.info('Email Mongo persistence disabled.')
    return

  db_name = os.environ.get('MONGO_DATABASE', 'notification_db')
  coll_name = os.environ.get('MONGO_EMAIL_EVENTS_COLLECTION', 'email_events')

  try:
    mongo_client = MongoClient(mongo_uri, serverSelectionTimeoutMS=5000)
    mongo_client.admin.command('ping')
    email_events_collection = mongo_client[db_name][coll_name]
    email_events_collection.create_index([('orderId', 1)], name='idx_order_id')
    email_events_collection.create_index([('createdAt', 1)], name='idx_created_at')
    logger.info(f'Email Mongo persistence enabled on {db_name}.{coll_name}')
  except Exception as exc:
    logger.warning(f'Email Mongo initialization failed, continuing without persistence: {exc}')
    mongo_client = None
    email_events_collection = None

app = Flask(__name__)

@app.route('/send-confirmation', methods=['POST'])
def send_order_confirmation():
    data = request.get_json()
    email = data.get('email', '')
    order = data.get('order', {})
    order_id = order.get('orderId', '')
    logger.info('A request to send order confirmation email to {} has been received.'.format(email))

    if email_events_collection is not None:
      try:
        email_events_collection.insert_one({
          'orderId': order_id,
          'email': email,
          'status': 'accepted',
          'template': 'confirmation.html',
          'requestPayload': {
            'orderId': order_id,
            'shippingTrackingId': order.get('shippingTrackingId', ''),
            'itemCount': len(order.get('items', [])),
          },
          'createdAt': datetime.utcnow(),
        })
      except Exception as exc:
        logger.warning(f'Failed to persist email event: {exc}')

    return jsonify({})

@app.route('/_healthz', methods=['GET'])
def health_check():
    return 'ok'

def initStackdriverProfiling():
  project_id = None
  try:
    project_id = os.environ["GCP_PROJECT_ID"]
  except KeyError:
    pass
  return


if __name__ == '__main__':
  logger.info('starting the email service in dummy mode.')
  init_mongo_store()

  try:
    if "DISABLE_PROFILER" in os.environ:
      raise KeyError()
    else:
      logger.info("Profiler enabled.")
      initStackdriverProfiling()
  except KeyError:
      logger.info("Profiler disabled.")

  port = os.environ.get('PORT', "8080")
  logger.info("listening on port: " + port)
  app.run(host='0.0.0.0', port=int(port))
