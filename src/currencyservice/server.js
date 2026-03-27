/*
 *
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 */

const pino = require('pino');
const logger = pino({
  name: 'currencyservice-server',
  messageKey: 'message',
  formatters: {
    level (logLevelString, logLevelNum) {
      return { severity: logLevelString }
    }
  }
});

if(process.env.DISABLE_PROFILER) {
  logger.info("Profiler disabled.")
}
else {
  logger.info("Profiler enabled.")
  require('@google-cloud/profiler').start({
    serviceContext: {
      service: 'currencyservice',
      version: '1.0.0'
    }
  });
}

const express = require('express');

const PORT = process.env.PORT || '7000';

/**
 * Helper function that gets currency data from a stored JSON file
 * Uses public data from European Central Bank
 */
function _getCurrencyData (callback) {
  const data = require('./data/currency_conversion.json');
  callback(data);
}

/**
 * Helper function that handles decimal/fractional carrying
 */
function _carry (amount) {
  const fractionSize = Math.pow(10, 9);
  amount.nanos += (amount.units % 1) * fractionSize;
  amount.units = Math.floor(amount.units) + Math.floor(amount.nanos / fractionSize);
  amount.nanos = amount.nanos % fractionSize;
  return amount;
}

/**
 * Lists the supported currencies
 */
function getSupportedCurrencies (req, res) {
  logger.info('Getting supported currencies...');
  _getCurrencyData((data) => {
    res.json({currencyCodes: Object.keys(data)});
  });
}

/**
 * Converts between currencies
 */
function convert (req, res) {
  try {
    _getCurrencyData((data) => {
      const request = req.body;

      const from = request.from;
      const euros = _carry({
        units: from.units / data[from.currencyCode],
        nanos: from.nanos / data[from.currencyCode]
      });

      euros.nanos = Math.round(euros.nanos);

      const result = _carry({
        units: euros.units * data[request.toCode],
        nanos: euros.nanos * data[request.toCode]
      });

      result.units = Math.floor(result.units);
      result.nanos = Math.floor(result.nanos);
      result.currencyCode = request.toCode;

      logger.info(`conversion request successful`);
      res.json(result);
    });
  } catch (err) {
    logger.error(`conversion request failed: ${err}`);
    res.status(500).json({ error: err.message });
  }
}

/**
 * Endpoint for health checks
 */
function check (req, res) {
  res.send('ok');
}

/**
 * Starts an HTTP server that receives requests for the
 * CurrencyConverter service at the sample server port
 */
function main () {
  logger.info(`Starting REST server on port ${PORT}...`);
  const app = express();
  app.use(express.json());

  app.get('/currencies', getSupportedCurrencies);
  app.post('/convert', convert);
  app.get('/_healthz', check);

  app.listen(PORT, () => {
    logger.info(`CurrencyService REST server started on port ${PORT}`);
  });
}

main();
