'use strict';

/**
 * Razorpay Payment Module
 *
 * Provides:
 *   createRazorpayOrder({ amount, currency })
 *     → { orderId, amount, currency, keyId }
 *
 *   verifyRazorpayPayment({ razorpay_order_id, razorpay_payment_id, razorpay_signature })
 *     → { transactionId }
 *
 *   charge(request)  [legacy no-op — payment already collected via Razorpay popup]
 *     → { transactionId }
 *
 * Environment Variables:
 *   RAZORPAY_KEY_ID     — Razorpay Key ID  (rzp_test_...)
 *   RAZORPAY_KEY_SECRET — Razorpay Key Secret
 */

const crypto = require('crypto');
const { v4: uuidv4 } = require('uuid');
const pino = require('pino');

const logger = pino({
  name: 'paymentservice-razorpay',
  messageKey: 'message',
  formatters: {
    level(logLevelString) {
      return { severity: logLevelString };
    },
  },
});

// ── Razorpay initialisation ──────────────────────────────────────────────────

const RAZORPAY_KEY_ID = process.env.RAZORPAY_KEY_ID;
const RAZORPAY_KEY_SECRET = process.env.RAZORPAY_KEY_SECRET;

if (!RAZORPAY_KEY_ID || !RAZORPAY_KEY_SECRET) {
  logger.warn(
    'RAZORPAY_KEY_ID or RAZORPAY_KEY_SECRET is not set — Razorpay payments will fail at runtime'
  );
}

// Lazy-initialise Razorpay client so health checks pass even if keys are absent.
let _razorpay = null;
function getRazorpay() {
  if (!_razorpay) {
    if (!RAZORPAY_KEY_ID || !RAZORPAY_KEY_SECRET) {
      throw new PaymentError(
        'Payment processor not configured (missing RAZORPAY_KEY_ID or RAZORPAY_KEY_SECRET)'
      );
    }
    const Razorpay = require('razorpay');
    _razorpay = new Razorpay({
      key_id: RAZORPAY_KEY_ID,
      key_secret: RAZORPAY_KEY_SECRET,
    });
  }
  return _razorpay;
}

// ── Custom error class ───────────────────────────────────────────────────────

class PaymentError extends Error {
  constructor(message) {
    super(message);
    this.code = 400;
  }
}

// ── Create Razorpay Order ────────────────────────────────────────────────────

/**
 * Creates a Razorpay order.
 *
 * @param {{ amount: number, currency: string }} params
 *   amount   — in smallest currency unit (paise for INR, cents for USD)
 *   currency — ISO 4217 code, e.g. 'INR'
 * @returns {Promise<{ orderId: string, amount: number, currency: string, keyId: string }>}
 */
async function createRazorpayOrder({ amount, currency }) {
  const razorpay = getRazorpay();
  const normalizedCurrency = (currency || 'INR').toUpperCase();
  const receiptId = `rcpt_${Date.now()}`;

  logger.info(
    { amount, currency: normalizedCurrency, receiptId },
    'PaymentService#CreateOrder — creating Razorpay order'
  );

  let order;
  try {
    order = await razorpay.orders.create({
      amount,
      currency: normalizedCurrency,
      receipt: receiptId,
      payment_capture: 1, // auto-capture on success
    });
  } catch (err) {
    logger.error(
      {
        razorpayError: err.message,
        razorpayDescription: err.error?.description,
        statusCode: err.statusCode,
      },
      'PaymentService#CreateOrder — Razorpay API call failed'
    );
    throw new PaymentError(
      `Failed to create payment order: ${err.error?.description || err.message || 'unknown error'}`
    );
  }

  logger.info(
    { orderId: order.id, amount: order.amount, currency: order.currency },
    'PaymentService#CreateOrder — Razorpay order created successfully'
  );

  return {
    orderId: order.id,
    amount: order.amount,
    currency: order.currency,
    keyId: RAZORPAY_KEY_ID,
  };
}

// ── Verify Razorpay Payment ──────────────────────────────────────────────────

/**
 * Verifies the HMAC-SHA256 signature sent by Razorpay's success callback.
 * The signature is computed as:
 *   HMAC-SHA256( razorpay_order_id + "|" + razorpay_payment_id, key_secret )
 *
 * @param {{ razorpay_order_id: string, razorpay_payment_id: string, razorpay_signature: string }} params
 * @returns {{ transactionId: string }}
 */
function verifyRazorpayPayment({ razorpay_order_id, razorpay_payment_id, razorpay_signature }) {
  if (!razorpay_order_id || !razorpay_payment_id || !razorpay_signature) {
    throw new PaymentError('Missing required Razorpay payment fields for verification');
  }

  if (!RAZORPAY_KEY_SECRET) {
    throw new PaymentError(
      'Payment processor not configured (missing RAZORPAY_KEY_SECRET)'
    );
  }

  logger.info(
    { razorpay_order_id, razorpay_payment_id },
    'PaymentService#VerifyPayment — verifying Razorpay HMAC signature'
  );

  const body = `${razorpay_order_id}|${razorpay_payment_id}`;
  const expectedSignature = crypto
    .createHmac('sha256', RAZORPAY_KEY_SECRET)
    .update(body)
    .digest('hex');

  // Constant-time comparison to prevent timing attacks
  const sigBuffer = Buffer.from(razorpay_signature, 'hex');
  const expBuffer = Buffer.from(expectedSignature, 'hex');

  const isValid =
    sigBuffer.length === expBuffer.length &&
    crypto.timingSafeEqual(sigBuffer, expBuffer);

  if (!isValid) {
    logger.error(
      { razorpay_order_id, razorpay_payment_id },
      'PaymentService#VerifyPayment — signature mismatch, possible payment tampering'
    );
    throw new PaymentError('Payment verification failed: invalid signature');
  }

  logger.info(
    { transactionId: razorpay_payment_id, razorpay_order_id },
    'PaymentService#VerifyPayment — signature verified, payment confirmed'
  );

  return { transactionId: razorpay_payment_id };
}

// ── Legacy /charge compatibility ─────────────────────────────────────────────

/**
 * Legacy charge endpoint handler.
 * Payment has already been collected via the Razorpay popup by the time
 * this is called. Returns a synthetic transactionId for any internal
 * callers that still hit POST /charge.
 *
 * @returns {Promise<{ transactionId: string }>}
 */
async function charge(_request) {
  logger.info(
    'PaymentService#Charge — legacy endpoint called (payment already processed via Razorpay)'
  );
  return { transactionId: uuidv4() };
}

module.exports = { createRazorpayOrder, verifyRazorpayPayment, charge };
