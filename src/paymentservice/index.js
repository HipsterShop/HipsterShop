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
const { MongoClient } = require('mongodb')

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

let paymentEventsCollection = null;

async function initPaymentStore() {
  const mongoUri = process.env.PAYMENT_MONGO_URI || process.env.MONGO_URI;
  if (!mongoUri) {
    logger.info('Payment Mongo persistence disabled.');
    return;
  }

  const dbName = process.env.MONGO_DATABASE || 'payment_db';
  const collName = process.env.MONGO_PAYMENTS_COLLECTION || 'charges';

  try {
    const client = new MongoClient(mongoUri, { serverSelectionTimeoutMS: 5000 });
    await client.connect();
    paymentEventsCollection = client.db(dbName).collection(collName);
    await paymentEventsCollection.createIndex({ transactionId: 1 }, { name: 'idx_transaction_id' });
    await paymentEventsCollection.createIndex({ createdAt: 1 }, { name: 'idx_created_at' });
    logger.info(`Payment Mongo persistence enabled on ${dbName}.${collName}`);
  } catch (err) {
    logger.warn({ err: err.message }, 'Payment Mongo initialization failed; continuing without persistence');
    paymentEventsCollection = null;
  }
}

function maskCard(cardNumber) {
  const digits = String(cardNumber || '').replace(/\D/g, '');
  if (!digits) {
    return '';
  }
  return `****${digits.slice(-4)}`;
}

async function persistChargeEvent(req, response, status, errorMessage) {
  if (!paymentEventsCollection) {
    return;
  }

  const amount = req.body?.amount || {};
  const card = req.body?.creditCard || {};
  const doc = {
    transactionId: response?.transactionId || null,
    requestId: req.headers['x-request-id'] || null,
    status,
    error: errorMessage || null,
    amount: {
      currencyCode: amount.currencyCode || null,
      units: amount.units ?? null,
      nanos: amount.nanos ?? null,
    },
    cardMeta: {
      last4Masked: maskCard(card.creditCardNumber),
      expirationMonth: card.creditCardExpirationMonth || null,
      expirationYear: card.creditCardExpirationYear || null,
    },
    createdAt: new Date(),
  };

  try {
    await paymentEventsCollection.insertOne(doc);
  } catch (err) {
    logger.warn({ err: err.message }, 'Failed to persist payment charge event');
  }
}

app.post('/charge', async (req, res) => {
  try {
    logger.info(`PaymentService#Charge invoked with request ${JSON.stringify(req.body)}`);
    const response = charge(req.body);
    await persistChargeEvent(req, response, 'success', null);
    res.json(response);
  } catch (err) {
    console.warn(err);
    await persistChargeEvent(req, null, 'failed', err.message);
    res.status(400).json({ error: err.message });
  }
});

app.get('/_healthz', (req, res) => {
  res.send('ok');
});

initPaymentStore()
  .catch((err) => {
    logger.warn({ err: err.message }, 'Payment Mongo setup failed; continuing without persistence');
  })
  .finally(() => {
    app.listen(PORT, () => {
      logger.info(`PaymentService REST server started on port ${PORT}`);
    });
  });
