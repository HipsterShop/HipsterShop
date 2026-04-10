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

const logger = require('./logger');
const { MongoClient } = require('mongodb');

if (process.env.DISABLE_PROFILER) {
  logger.info('Profiler disabled.');
} else {
  logger.info('Profiler enabled.');
  require('@google-cloud/profiler').start({
    serviceContext: {
      service: 'paymentservice',
      version: '1.0.0',
    },
  });
}

const express = require('express');
const { createRazorpayOrder, verifyRazorpayPayment, charge } = require('./charge');

const PORT = process.env['PORT'] || '50051';

const app = express();
app.use(express.json());

// ── MongoDB persistence (optional) ───────────────────────────────────────────

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
  if (!digits) return '';
  return `****${digits.slice(-4)}`;
}

async function persistChargeEvent(req, response, status, errorMessage) {
  if (!paymentEventsCollection) return;

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

async function persistRazorpayEvent(type, data, status, errorMessage) {
  if (!paymentEventsCollection) return;

  const doc = {
    type,
    transactionId: data?.transactionId || data?.razorpay_payment_id || null,
    orderId: data?.orderId || data?.razorpay_order_id || null,
    status,
    error: errorMessage || null,
    createdAt: new Date(),
  };

  try {
    await paymentEventsCollection.insertOne(doc);
  } catch (err) {
    logger.warn({ err: err.message }, 'Failed to persist Razorpay event');
  }
}

// ── Routes ───────────────────────────────────────────────────────────────────

/**
 * POST /create-order
 * Creates a Razorpay order and returns credentials for the frontend popup.
 *
 * Request body:
 *   { amount: number, currency: string }
 *   amount — in smallest currency unit (paise for INR, cents for USD)
 *
 * Response:
 *   { orderId, amount, currency, keyId }
 */
app.post('/create-order', async (req, res) => {
  try {
    const { amount, currency } = req.body;

    if (!amount || typeof amount !== 'number' || amount <= 0) {
      return res.status(400).json({ error: 'Invalid amount: must be a positive number in smallest currency unit' });
    }

    logger.info(
      { amount, currency },
      'PaymentService#CreateOrder — received create-order request'
    );

    const result = await createRazorpayOrder({ amount, currency });
    await persistRazorpayEvent('create-order', result, 'success', null);
    res.json(result);
  } catch (err) {
    logger.error({ err: err.message }, 'PaymentService#CreateOrder — failed');
    await persistRazorpayEvent('create-order', {}, 'failed', err.message);
    res.status(400).json({ error: err.message });
  }
});

/**
 * POST /verify
 * Verifies the HMAC-SHA256 payment signature from Razorpay's success callback.
 *
 * Request body:
 *   { razorpay_order_id, razorpay_payment_id, razorpay_signature }
 *
 * Response:
 *   { transactionId }  — razorpay_payment_id on success
 */
app.post('/verify', (req, res) => {
  try {
    const { razorpay_order_id, razorpay_payment_id, razorpay_signature } = req.body;

    logger.info(
      { razorpay_order_id, razorpay_payment_id },
      'PaymentService#Verify — received payment verification request'
    );

    const result = verifyRazorpayPayment({
      razorpay_order_id,
      razorpay_payment_id,
      razorpay_signature,
    });

    persistRazorpayEvent('verify', { ...req.body, transactionId: result.transactionId }, 'success', null);
    res.json(result);
  } catch (err) {
    logger.error({ err: err.message }, 'PaymentService#Verify — failed');
    persistRazorpayEvent('verify', req.body || {}, 'failed', err.message);
    res.status(400).json({ error: err.message });
  }
});

/**
 * POST /charge
 * Legacy endpoint — payment is already collected via Razorpay popup.
 * Returns a synthetic transactionId for backward compatibility.
 */
app.post('/charge', async (req, res) => {
  try {
    logger.info(`PaymentService#Charge (legacy) invoked with request ${JSON.stringify(req.body)}`);
    const response = await charge(req.body);
    await persistChargeEvent(req, response, 'success', null);
    res.json(response);
  } catch (err) {
    console.warn(err);
    await persistChargeEvent(req, null, 'failed', err.message);
    res.status(400).json({ error: err.message });
  }
});

/**
 * GET /_healthz
 */
app.get('/_healthz', (req, res) => {
  res.send('ok');
});

// ── Boot ─────────────────────────────────────────────────────────────────────

initPaymentStore()
  .catch((err) => {
    logger.warn({ err: err.message }, 'Payment Mongo setup failed; continuing without persistence');
  })
  .finally(() => {
    app.listen(PORT, () => {
      logger.info(`PaymentService REST server started on port ${PORT}`);
    });
  });
