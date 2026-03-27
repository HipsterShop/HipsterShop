
import os
import sys
import time
import traceback
import json
from flask import Flask, request, jsonify
from jinja2 import Environment, FileSystemLoader, select_autoescape, TemplateError
from google.auth.exceptions import DefaultCredentialsError

from logger import getJSONLogger
logger = getJSONLogger('emailservice-server')

env = Environment(
    loader=FileSystemLoader('templates'),
    autoescape=select_autoescape(['html', 'xml'])
)
template = env.get_template('confirmation.html')

app = Flask(__name__)

@app.route('/send-confirmation', methods=['POST'])
def send_order_confirmation():
    data = request.get_json()
    email = data.get('email', '')
    order = data.get('order', {})
    logger.info('A request to send order confirmation email to {} has been received.'.format(email))
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
