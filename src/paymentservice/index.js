/*
 *
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 */

'use strict';

const logger = require('./logger')

if (process.env.DISABLE_PROFILER) {
  logger.info("Profiler disabled.")
} else {
  logger.info("Profiler enabled.")
  require('@google-cloud/profiler').start({
    serviceContext: {
      service: 'paymentservice',
      version: '1.0.0'
    }
  });
}

const express = require('express');
const charge = require('./charge');

const PORT = process.env['PORT'] || '50051';

const app = express();
app.use(express.json());

app.post('/charge', (req, res) => {
  try {
    logger.info(`PaymentService#Charge invoked with request ${JSON.stringify(req.body)}`);
    const response = charge(req.body);
    res.json(response);
  } catch (err) {
    console.warn(err);
    res.status(400).json({ error: err.message });
  }
});

app.get('/_healthz', (req, res) => {
  res.send('ok');
});

app.listen(PORT, () => {
  logger.info(`PaymentService REST server started on port ${PORT}`);
});
