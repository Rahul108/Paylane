'use client';

import React, { useState } from 'react';

interface ServiceNode {
  name: string;
  port: number;
  role: string;
  status: 'ONLINE' | 'STANDBY';
}

const SERVICES: ServiceNode[] = [
  { name: 'web', port: 3010, role: 'Frontend + BFF + Test Console', status: 'ONLINE' },
  { name: 'orchestrator', port: 4010, role: 'Journey Saga & Step Tracker', status: 'ONLINE' },
  { name: 'payment-core', port: 4011, role: 'State Machine & Double-Entry Ledger', status: 'ONLINE' },
  { name: 'mock-mfs', port: 5011, role: 'Mock MFS PGW (bKash-like)', status: 'ONLINE' },
  { name: 'mock-card', port: 5012, role: 'Mock Card PGW (3DS OTP & Tokens)', status: 'ONLINE' },
  { name: 'mock-downstream', port: 5013, role: 'Mock Telecom Recharge / Cashback', status: 'ONLINE' },
];

type JourneyType = 'ui_payment' | 'uiless_payment' | 'payment_recharge' | 'payment_cashback' | 'payment_subscription';
type PaymentMethod = 'MFS' | 'Card';
type JourneyStatus = 'IDLE' | 'STARTED' | 'PAYMENT' | 'TOKEN_CHARGE' | 'DOWNSTREAM' | 'COMPLETED' | 'FAILED' | 'NEEDS_ATTENTION';
type PaymentState = 'IDLE' | 'CREATED' | 'PENDING' | 'AUTHORIZED' | 'CAPTURED' | 'FAILED' | 'REFUNDED';
type StepStatus = 'PENDING' | 'RUNNING' | 'SUCCEEDED' | 'FAILED' | 'RETRYING' | 'NEEDS_ATTENTION';

interface JourneyStep {
  name: string;
  status: StepStatus;
  attempts: number;
  details?: string;
  error?: string;
}

interface LedgerEntry {
  type: 'DEBIT' | 'CREDIT';
  account: string;
  amount: number;
  currency: string;
  reason: string;
}

interface StepperStage {
  key: string;
  label: string;
  desc: string;
}

export default function HomePage() {
  const [selectedJourney, setSelectedJourney] = useState<JourneyType>('payment_recharge');
  const [paymentMethod, setPaymentMethod] = useState<PaymentMethod>('MFS');
  const [amount, setAmount] = useState<number>(500);
  const [scenario, setScenario] = useState<string>('SUCCESS');

  // Journey-specific parameters
  const [mobileNumber, setMobileNumber] = useState<string>('01712345678');
  const [operator, setOperator] = useState<string>('Operator A');
  const [promoCode, setPromoCode] = useState<string>('SAVE10');
  const [planName, setPlanName] = useState<string>('PREMIUM_MONTHLY');
  const [boundToken, setBoundToken] = useState<string>('agr_mfs_88017000');

  // Active execution state
  const [journeyId, setJourneyId] = useState<string | null>(null);
  const [paymentId, setPaymentId] = useState<string | null>(null);
  const [currentStageIndex, setCurrentStageIndex] = useState<number>(0);
  const [journeyStatus, setJourneyStatus] = useState<JourneyStatus>('IDLE');
  const [paymentState, setPaymentState] = useState<PaymentState>('IDLE');
  const [steps, setSteps] = useState<JourneyStep[]>([]);
  const [ledgerEntries, setLedgerEntries] = useState<LedgerEntry[]>([]);
  const [eventLogs, setEventLogs] = useState<string[]>([]);
  const [isProcessing, setIsProcessing] = useState<boolean>(false);

  const log = (msg: string) => {
    const time = new Date().toLocaleTimeString();
    setEventLogs((prev) => [`[${time}] ${msg}`, ...prev.slice(0, 30)]);
  };

  const generateULID = (prefix: string) => {
    const ts = Date.now().toString(36).toUpperCase();
    const rand = Math.random().toString(36).substring(2, 9).toUpperCase();
    return `${prefix}_${ts}${rand}`;
  };

  // Define unique stepper stages per journey
  const getJourneyStages = (type: JourneyType): StepperStage[] => {
    switch (type) {
      case 'ui_payment':
        return [
          { key: 'STARTED', label: '1. INITIATION', desc: 'BFF ➔ orchestrator' },
          { key: 'PAYMENT', label: '2. HOSTED PGW', desc: 'User OTP & PIN Approval' },
          { key: 'COMPLETED', label: '3. COMPLETED', desc: 'Payment Captured' },
        ];
      case 'uiless_payment':
        return [
          { key: 'STARTED', label: '1. INITIATION', desc: 'BFF ➔ orchestrator' },
          { key: 'TOKEN_CHARGE', label: '2. BOUND CHARGE', desc: 'Direct PGW Token Charge' },
          { key: 'COMPLETED', label: '3. COMPLETED', desc: 'Zero-Redirect Capture' },
        ];
      case 'payment_recharge':
        return [
          { key: 'STARTED', label: '1. INITIATION', desc: 'BFF ➔ orchestrator' },
          { key: 'PAYMENT', label: '2. PAYMENT', desc: 'Hosted PGW Checkout' },
          { key: 'RECHARGE', label: '3. RECHARGE', desc: 'mock-downstream Telecom' },
          { key: 'COMPLETED', label: '4. COMPLETED', desc: 'Recharge Dispatched' },
        ];
      case 'payment_cashback':
        return [
          { key: 'STARTED', label: '1. INITIATION', desc: 'BFF ➔ orchestrator' },
          { key: 'PAYMENT', label: '2. PAYMENT', desc: 'Hosted PGW Checkout' },
          { key: 'CASHBACK', label: '3. CASHBACK', desc: 'mock-downstream Grant' },
          { key: 'COMPLETED', label: '4. COMPLETED', desc: 'Cashback Credited' },
        ];
      case 'payment_subscription':
        return [
          { key: 'STARTED', label: '1. INITIATION', desc: 'BFF ➔ orchestrator' },
          { key: 'PAYMENT', label: '2. PAYMENT', desc: 'Hosted PGW Checkout' },
          { key: 'SUBSCRIBE', label: '3. SUBSCRIBE', desc: 'mock-downstream Activation' },
          { key: 'COMPLETED', label: '4. COMPLETED', desc: 'Plan Activated' },
        ];
    }
  };

  // Define unique steps table per journey
  const getInitialSteps = (type: JourneyType): JourneyStep[] => {
    switch (type) {
      case 'ui_payment':
        return [{ name: 'PAYMENT (Redirect)', status: 'PENDING', attempts: 0 }];
      case 'uiless_payment':
        return [{ name: 'TOKEN_CHARGE (UI-less)', status: 'PENDING', attempts: 0 }];
      case 'payment_recharge':
        return [
          { name: 'PAYMENT (Redirect)', status: 'PENDING', attempts: 0 },
          { name: 'RECHARGE (Downstream)', status: 'PENDING', attempts: 0, details: `${operator} - ${mobileNumber}` }
        ];
      case 'payment_cashback':
        return [
          { name: 'PAYMENT (Redirect)', status: 'PENDING', attempts: 0 },
          { name: 'CASHBACK (Downstream)', status: 'PENDING', attempts: 0, details: `Promo: ${promoCode}` }
        ];
      case 'payment_subscription':
        return [
          { name: 'PAYMENT (Redirect)', status: 'PENDING', attempts: 0 },
          { name: 'SUBSCRIBE (Downstream)', status: 'PENDING', attempts: 0, details: `Plan: ${planName}` }
        ];
    }
  };

  const startJourney = () => {
    const newJourneyId = generateULID('JRN');
    const newPaymentId = generateULID('PAY');
    setJourneyId(newJourneyId);
    setPaymentId(newPaymentId);
    setLedgerEntries([]);
    setEventLogs([]);
    setIsProcessing(true);

    const initialSteps = getInitialSteps(selectedJourney);
    setSteps(initialSteps);
    setJourneyStatus('STARTED');
    setPaymentState('CREATED');
    setCurrentStageIndex(1); // 1. INITIATION

    log(`[${selectedJourney}] Journey initialized with ID: ${newJourneyId}`);
    log(`BFF sealed request with JWE (RS256 sign + RSA-OAEP-256 enc) ➔ orchestrator:4010`);
    log(`Orchestrator called payment-core:4011 -> Payment Created (Status: CREATED)`);

    if (selectedJourney === 'uiless_payment') {
      // UI-LESS DIRECT FLOW: No browser redirect, charge bound token immediately!
      setTimeout(() => {
        setCurrentStageIndex(2);
        setJourneyStatus('TOKEN_CHARGE');
        setPaymentState('PENDING');
        setSteps((s) => s.map((step) => ({ ...step, status: 'RUNNING', attempts: 1 })));
        log(`[UI-less] Charging bound agreement '${boundToken}' directly via adapter without browser redirect`);

        setTimeout(() => {
          if (scenario === 'SUCCESS') {
            setPaymentState('CAPTURED');
            setLedgerEntries([
              { type: 'DEBIT', account: `customer_token_${paymentMethod.toLowerCase()}`, amount: amount * 100, currency: 'BDT', reason: 'TOKEN_CHARGE' },
              { type: 'CREDIT', account: 'merchant_settlement', amount: amount * 100, currency: 'BDT', reason: 'PAYMENT_SETTLEMENT' }
            ]);
            setSteps((s) => s.map((step) => ({ ...step, status: 'SUCCEEDED', details: 'Charged via saved agreement' })));
            setCurrentStageIndex(3);
            setJourneyStatus('COMPLETED');
            setIsProcessing(false);
            log(`[UI-less] Adapter synchronous charge SUCCEEDED! Payment CAPTURED.`);
            log(`Journey ${newJourneyId} COMPLETED.`);
          } else {
            setPaymentState('FAILED');
            setSteps((s) => s.map((step) => ({ ...step, status: 'FAILED', error: 'DECLINED_BY_PROVIDER' })));
            setJourneyStatus('FAILED');
            setIsProcessing(false);
            log(`[UI-less] Token charge declined by provider.`);
          }
        }, 1600);
      }, 1200);

    } else if (selectedJourney === 'ui_payment') {
      // UI-PAYMENT ONLY: Payment redirect, but no downstream actions
      setTimeout(() => {
        setCurrentStageIndex(2);
        setJourneyStatus('PAYMENT');
        setPaymentState('PENDING');
        setSteps((s) => s.map((step) => ({ ...step, status: 'RUNNING', attempts: 1 })));
        log(`Redirected to mock-${paymentMethod.toLowerCase()}:501${paymentMethod === 'MFS' ? '1' : '2'} hosted page`);

        setTimeout(() => {
          if (scenario === 'SUCCESS') {
            setPaymentState('CAPTURED');
            setLedgerEntries([
              { type: 'DEBIT', account: `customer_${paymentMethod.toLowerCase()}`, amount: amount * 100, currency: 'BDT', reason: 'UI_PAYMENT_CAPTURE' },
              { type: 'CREDIT', account: 'merchant_settlement', amount: amount * 100, currency: 'BDT', reason: 'PAYMENT_SETTLEMENT' }
            ]);
            setSteps((s) => s.map((step) => ({ ...step, status: 'SUCCEEDED', details: 'Hosted checkout completed' })));
            setCurrentStageIndex(3);
            setJourneyStatus('COMPLETED');
            setIsProcessing(false);
            log(`Payment webhook received and verified! Payment CAPTURED.`);
            log(`No downstream steps required for ui_payment. Redirecting back to merchant return URL.`);
            log(`Journey ${newJourneyId} COMPLETED.`);
          } else {
            setPaymentState('FAILED');
            setSteps((s) => s.map((step) => ({ ...step, status: 'FAILED', error: scenario })));
            setJourneyStatus('FAILED');
            setIsProcessing(false);
            log(`Payment failed: ${scenario}`);
          }
        }, 1800);
      }, 1200);

    } else {
      // MULTI-STEP FLOWS: Payment redirect + Downstream action (Recharge, Cashback, or Subscription)
      setTimeout(() => {
        setCurrentStageIndex(2);
        setJourneyStatus('PAYMENT');
        setPaymentState('PENDING');
        setSteps((s) => s.map((step, idx) => idx === 0 ? { ...step, status: 'RUNNING', attempts: 1 } : step));
        log(`Hosted PGW checkout session opened for ${paymentMethod}`);

        setTimeout(() => {
          if (scenario === 'SUCCESS') {
            setPaymentState('CAPTURED');
            setLedgerEntries([
              { type: 'DEBIT', account: `customer_${paymentMethod.toLowerCase()}`, amount: amount * 100, currency: 'BDT', reason: 'PAYMENT_CAPTURE' },
              { type: 'CREDIT', account: 'merchant_settlement', amount: amount * 100, currency: 'BDT', reason: 'PAYMENT_SETTLEMENT' }
            ]);
            setSteps((s) => s.map((step, idx) => idx === 0 ? { ...step, status: 'SUCCEEDED' } : step));
            log(`Payment CAPTURED. Outbox event dispatched to orchestrator.`);

            // Progress to Stage 3: Downstream action
            setCurrentStageIndex(3);
            setJourneyStatus('DOWNSTREAM');
            setSteps((s) => s.map((step, idx) => idx === 1 ? { ...step, status: 'RUNNING', attempts: 1 } : step));

            const downstreamName = selectedJourney === 'payment_recharge' ? 'RECHARGE' : selectedJourney === 'payment_cashback' ? 'CASHBACK' : 'SUBSCRIBE';
            log(`Orchestrator invoking mock-downstream:5013 for step '${downstreamName}' with idempotency key '${newJourneyId}_${downstreamName}'`);

            setTimeout(() => {
              let detailMsg = '';
              if (selectedJourney === 'payment_recharge') detailMsg = `Recharged ${amount} BDT to ${mobileNumber} (${operator})`;
              else if (selectedJourney === 'payment_cashback') detailMsg = `Granted 10% Cashback (${(amount * 0.1).toFixed(2)} BDT) under promo '${promoCode}'`;
              else if (selectedJourney === 'payment_subscription') detailMsg = `Activated '${planName}' for customer`;

              setSteps((s) => s.map((step, idx) => idx === 1 ? { ...step, status: 'SUCCEEDED', details: detailMsg } : step));
              setCurrentStageIndex(4);
              setJourneyStatus('COMPLETED');
              setIsProcessing(false);
              log(`Downstream step '${downstreamName}' finished: ${detailMsg}`);
              log(`Multi-step saga ${newJourneyId} COMPLETED.`);
            }, 1800);

          } else {
            setPaymentState('FAILED');
            setSteps((s) => s.map((step, idx) => idx === 0 ? { ...step, status: 'FAILED', error: scenario } : step));
            setJourneyStatus('FAILED');
            setIsProcessing(false);
            log(`Payment failed: ${scenario}. Downstream step skipped.`);
          }
        }, 1800);
      }, 1200);
    }
  };

  const resetJourney = () => {
    setJourneyId(null);
    setPaymentId(null);
    setCurrentStageIndex(0);
    setJourneyStatus('IDLE');
    setPaymentState('IDLE');
    setSteps([]);
    setLedgerEntries([]);
    setEventLogs([]);
    setIsProcessing(false);
  };

  const activeStages = getJourneyStages(selectedJourney);

  return (
    <main style={{ padding: '2rem 3rem', maxWidth: '1300px', margin: '0 auto', fontFamily: 'inherit' }}>
      {/* Header */}
      <header style={{ marginBottom: '2.5rem', borderBottom: '1px solid var(--card-border)', paddingBottom: '1.5rem' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <div>
            <h1 style={{ fontSize: '2.2rem', fontWeight: 800, color: '#60a5fa', letterSpacing: '-0.02em' }}>Paylane</h1>
            <p style={{ color: '#94a3b8', marginTop: '0.4rem', fontSize: '1rem' }}>
              Autonomous Multi-Service Payment Platform • JWE-Secured Service Mesh
            </p>
          </div>
          <span style={{
            background: 'rgba(16, 185, 129, 0.15)',
            color: '#10b981',
            padding: '0.4rem 0.9rem',
            borderRadius: '9999px',
            fontSize: '0.85rem',
            fontWeight: 600,
            border: '1px solid rgba(16, 185, 129, 0.3)'
          }}>
            ● All 8 Services Active (127.0.0.1)
          </span>
        </div>
      </header>

      {/* Main Interactive Test Console */}
      <section style={{
        background: 'var(--card-bg)',
        border: '1px solid var(--card-border)',
        borderRadius: '1rem',
        padding: '2rem',
        boxShadow: '0 8px 32px rgba(0, 0, 0, 0.3)'
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem' }}>
          <div>
            <h2 style={{ fontSize: '1.4rem', fontWeight: 700 }}>Interactive Test Console</h2>
            <p style={{ color: '#94a3b8', fontSize: '0.9rem', marginTop: '0.2rem' }}>
              Experience the distinct workflow, step progression, and downstream logic for each journey.
            </p>
          </div>
          {journeyId && (
            <button
              onClick={resetJourney}
              style={{
                background: 'rgba(239, 68, 68, 0.15)',
                color: '#f87171',
                border: '1px solid rgba(239, 68, 68, 0.3)',
                padding: '0.4rem 0.8rem',
                borderRadius: '0.5rem',
                cursor: 'pointer',
                fontSize: '0.85rem',
                fontWeight: 600
              }}
            >
              Reset Console
            </button>
          )}
        </div>

        {/* Configuration Controls */}
        <div style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))',
          gap: '1.25rem',
          marginBottom: '1.5rem',
          padding: '1.25rem',
          background: 'rgba(0, 0, 0, 0.25)',
          borderRadius: '0.75rem',
          border: '1px solid rgba(255, 255, 255, 0.05)'
        }}>
          {/* Journey Selector */}
          <div>
            <label style={{ display: 'block', fontSize: '0.85rem', fontWeight: 600, color: '#cbd5e1', marginBottom: '0.5rem' }}>
              1. Choose Journey Type
            </label>
            <select
              value={selectedJourney}
              onChange={(e) => {
                setSelectedJourney(e.target.value as JourneyType);
                if (journeyId) resetJourney();
              }}
              disabled={isProcessing}
              style={{
                width: '100%',
                background: '#1e293b',
                color: '#f8fafc',
                border: '1px solid rgba(59, 130, 246, 0.4)',
                padding: '0.65rem 0.8rem',
                borderRadius: '0.5rem',
                fontSize: '0.9rem',
                fontWeight: 600
              }}
            >
              <option value="payment_recharge">payment_recharge (Payment ➔ Telecom Recharge)</option>
              <option value="ui_payment">ui_payment (Redirect Payment only)</option>
              <option value="uiless_payment">uiless_payment (Bound Token 1-Click Payment)</option>
              <option value="payment_cashback">payment_cashback (Payment ➔ Cashback Grant)</option>
              <option value="payment_subscription">payment_subscription (Payment ➔ Subscription)</option>
            </select>
          </div>

          {/* Payment Method */}
          <div>
            <label style={{ display: 'block', fontSize: '0.85rem', fontWeight: 600, color: '#cbd5e1', marginBottom: '0.5rem' }}>
              2. Payment Method
            </label>
            <div style={{ display: 'flex', gap: '0.5rem' }}>
              {(['MFS', 'Card'] as PaymentMethod[]).map((m) => (
                <button
                  key={m}
                  onClick={() => setPaymentMethod(m)}
                  disabled={isProcessing}
                  style={{
                    flex: 1,
                    background: paymentMethod === m ? '#3b82f6' : '#1e293b',
                    color: paymentMethod === m ? '#fff' : '#cbd5e1',
                    border: '1px solid ' + (paymentMethod === m ? '#3b82f6' : 'rgba(255,255,255,0.1)'),
                    padding: '0.65rem',
                    borderRadius: '0.5rem',
                    cursor: 'pointer',
                    fontWeight: 600,
                    fontSize: '0.9rem'
                  }}
                >
                  {m === 'MFS' ? '📱 MFS (bKash)' : '💳 Card (3DS)'}
                </button>
              ))}
            </div>
          </div>

          {/* Amount */}
          <div>
            <label style={{ display: 'block', fontSize: '0.85rem', fontWeight: 600, color: '#cbd5e1', marginBottom: '0.5rem' }}>
              3. Amount (BDT)
            </label>
            <input
              type="number"
              value={amount}
              onChange={(e) => setAmount(Number(e.target.value))}
              disabled={isProcessing}
              style={{
                width: '100%',
                background: '#1e293b',
                color: '#f8fafc',
                border: '1px solid rgba(255, 255, 255, 0.1)',
                padding: '0.65rem 0.8rem',
                borderRadius: '0.5rem',
                fontSize: '0.9rem'
              }}
            />
          </div>

          {/* Scenario */}
          <div>
            <label style={{ display: 'block', fontSize: '0.85rem', fontWeight: 600, color: '#cbd5e1', marginBottom: '0.5rem' }}>
              4. Scenario Outcome
            </label>
            <select
              value={scenario}
              onChange={(e) => setScenario(e.target.value)}
              disabled={isProcessing}
              style={{
                width: '100%',
                background: '#1e293b',
                color: '#f8fafc',
                border: '1px solid rgba(255, 255, 255, 0.1)',
                padding: '0.65rem 0.8rem',
                borderRadius: '0.5rem',
                fontSize: '0.9rem'
              }}
            >
              <option value="SUCCESS">Success Flow</option>
              <option value="INSUFFICIENT_BALANCE">Fail: Insufficient Balance</option>
              <option value="USER_ABANDONED">User Abandons Checkout</option>
            </select>
          </div>
        </div>

        {/* Journey-Specific Input Parameters Box */}
        <div style={{
          background: 'rgba(59, 130, 246, 0.08)',
          border: '1px solid rgba(59, 130, 246, 0.25)',
          borderRadius: '0.75rem',
          padding: '1.25rem',
          marginBottom: '2rem'
        }}>
          <div style={{ fontSize: '0.85rem', fontWeight: 700, color: '#60a5fa', marginBottom: '0.75rem' }}>
            ℹ Journey-Specific Parameters for [{selectedJourney}]
          </div>

          {selectedJourney === 'payment_recharge' && (
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem' }}>
              <div>
                <label style={{ fontSize: '0.8rem', color: '#94a3b8' }}>Mobile Number</label>
                <input
                  type="text"
                  value={mobileNumber}
                  onChange={(e) => setMobileNumber(e.target.value)}
                  disabled={isProcessing}
                  style={{ width: '100%', background: '#1e293b', color: '#fff', border: '1px solid rgba(255,255,255,0.1)', padding: '0.5rem', borderRadius: '0.375rem', marginTop: '0.2rem' }}
                />
              </div>
              <div>
                <label style={{ fontSize: '0.8rem', color: '#94a3b8' }}>Telecom Operator</label>
                <select
                  value={operator}
                  onChange={(e) => setOperator(e.target.value)}
                  disabled={isProcessing}
                  style={{ width: '100%', background: '#1e293b', color: '#fff', border: '1px solid rgba(255,255,255,0.1)', padding: '0.5rem', borderRadius: '0.375rem', marginTop: '0.2rem' }}
                >
                  <option value="Operator A">Operator A</option>
                  <option value="Operator B">Operator B</option>
                  <option value="Operator C">Operator C</option>
                  <option value="Operator D">Operator D</option>
                </select>
              </div>
            </div>
          )}

          {selectedJourney === 'payment_cashback' && (
            <div>
              <label style={{ fontSize: '0.8rem', color: '#94a3b8' }}>Campaign Promo Code</label>
              <input
                type="text"
                value={promoCode}
                onChange={(e) => setPromoCode(e.target.value)}
                disabled={isProcessing}
                style={{ width: '100%', maxWidth: '320px', background: '#1e293b', color: '#fff', border: '1px solid rgba(255,255,255,0.1)', padding: '0.5rem', borderRadius: '0.375rem', marginTop: '0.2rem' }}
              />
            </div>
          )}

          {selectedJourney === 'payment_subscription' && (
            <div>
              <label style={{ fontSize: '0.8rem', color: '#94a3b8' }}>Subscription Tier / Offer ID</label>
              <input
                type="text"
                value={planName}
                onChange={(e) => setPlanName(e.target.value)}
                disabled={isProcessing}
                style={{ width: '100%', maxWidth: '320px', background: '#1e293b', color: '#fff', border: '1px solid rgba(255,255,255,0.1)', padding: '0.5rem', borderRadius: '0.375rem', marginTop: '0.2rem' }}
              />
            </div>
          )}

          {selectedJourney === 'uiless_payment' && (
            <div>
              <label style={{ fontSize: '0.8rem', color: '#94a3b8' }}>Stored Token / Agreement Identifier (No user redirect)</label>
              <input
                type="text"
                value={boundToken}
                onChange={(e) => setBoundToken(e.target.value)}
                disabled={isProcessing}
                style={{ width: '100%', maxWidth: '400px', background: '#1e293b', color: '#fff', border: '1px solid rgba(255,255,255,0.1)', padding: '0.5rem', borderRadius: '0.375rem', marginTop: '0.2rem' }}
              />
            </div>
          )}

          {selectedJourney === 'ui_payment' && (
            <div style={{ fontSize: '0.85rem', color: '#cbd5e1' }}>
              Direct checkout flow without downstream steps. Completes with return redirect to customer portal.
            </div>
          )}
        </div>

        {/* Start Button */}
        <div style={{ marginBottom: '2.5rem' }}>
          <button
            onClick={startJourney}
            disabled={isProcessing}
            style={{
              background: isProcessing ? '#475569' : '#2563eb',
              color: '#ffffff',
              border: 'none',
              padding: '0.85rem 2.2rem',
              borderRadius: '0.5rem',
              fontSize: '1rem',
              fontWeight: 700,
              cursor: isProcessing ? 'not-allowed' : 'pointer',
              boxShadow: '0 4px 14px rgba(37, 99, 235, 0.4)',
              display: 'inline-flex',
              alignItems: 'center',
              gap: '0.6rem'
            }}
          >
            {isProcessing ? '⚡ Executing Journey...' : `▶ Start ${selectedJourney}`}
          </button>
        </div>

        {/* Real-Time Stepper & Detailed Results */}
        {journeyId && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '2rem' }}>
            {/* Dynamic Visual Stepper */}
            <div style={{
              background: 'rgba(0, 0, 0, 0.3)',
              padding: '1.75rem',
              borderRadius: '0.75rem',
              border: '1px solid rgba(255, 255, 255, 0.05)'
            }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.75rem' }}>
                <div>
                  <span style={{ fontSize: '1.05rem', fontWeight: 700, color: '#e2e8f0' }}>
                    Journey Progression Stepper: [{selectedJourney}]
                  </span>
                  <p style={{ fontSize: '0.8rem', color: '#94a3b8' }}>
                    Stages customized dynamically for this saga
                  </p>
                </div>
                <span style={{
                  padding: '0.35rem 0.85rem',
                  borderRadius: '9999px',
                  fontWeight: 700,
                  fontSize: '0.85rem',
                  background: journeyStatus === 'COMPLETED' ? 'rgba(16, 185, 129, 0.2)' : journeyStatus === 'FAILED' ? 'rgba(239, 68, 68, 0.2)' : 'rgba(59, 130, 246, 0.2)',
                  color: journeyStatus === 'COMPLETED' ? '#10b981' : journeyStatus === 'FAILED' ? '#ef4444' : '#60a5fa'
                }}>
                  {journeyStatus}
                </span>
              </div>

              {/* Stepper Dots & Line */}
              <div style={{ display: 'flex', alignItems: 'center', position: 'relative' }}>
                {activeStages.map((stage, idx) => {
                  const stageNum = idx + 1;
                  const isPassed = currentStageIndex > stageNum || (journeyStatus === 'COMPLETED' && stageNum === activeStages.length);
                  const isCurrent = currentStageIndex === stageNum && journeyStatus !== 'COMPLETED';
                  const isFail = journeyStatus === 'FAILED' && isCurrent;

                  return (
                    <React.Fragment key={stage.key}>
                      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', zIndex: 2, flex: 1, textAlign: 'center' }}>
                        <div style={{
                          width: '40px',
                          height: '40px',
                          borderRadius: '50%',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          fontWeight: 700,
                          fontSize: '0.9rem',
                          background: isFail ? '#ef4444' : isPassed ? '#10b981' : isCurrent ? '#3b82f6' : '#334155',
                          color: '#ffffff',
                          boxShadow: isCurrent ? '0 0 15px rgba(59, 130, 246, 0.7)' : 'none',
                          transition: 'all 0.3s ease'
                        }}>
                          {isPassed ? '✓' : stageNum}
                        </div>
                        <span style={{
                          fontSize: '0.85rem',
                          fontWeight: 700,
                          color: isCurrent ? '#60a5fa' : isPassed ? '#10b981' : '#94a3b8',
                          marginTop: '0.6rem'
                        }}>
                          {stage.label}
                        </span>
                        <span style={{ fontSize: '0.75rem', color: '#64748b', marginTop: '0.1rem' }}>{stage.desc}</span>
                      </div>
                      {idx < activeStages.length - 1 && (
                        <div style={{
                          flex: 1,
                          height: '3px',
                          background: currentStageIndex > stageNum ? '#10b981' : '#334155',
                          margin: '0 -25px 2.2rem -25px',
                          zIndex: 1,
                          transition: 'all 0.3s ease'
                        }} />
                      )}
                    </React.Fragment>
                  );
                })}
              </div>
            </div>

            {/* Steps & State Machine */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1.5rem' }}>
              {/* Distinct Steps Breakdown */}
              <div style={{
                background: 'rgba(0, 0, 0, 0.3)',
                padding: '1.25rem',
                borderRadius: '0.75rem',
                border: '1px solid rgba(255, 255, 255, 0.05)'
              }}>
                <h3 style={{ fontSize: '1rem', fontWeight: 700, marginBottom: '1rem', color: '#e2e8f0' }}>
                  Journey Steps ({steps.length})
                </h3>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
                  {steps.map((st) => (
                    <div
                      key={st.name}
                      style={{
                        padding: '0.85rem 1rem',
                        background: 'rgba(255, 255, 255, 0.03)',
                        borderRadius: '0.5rem',
                        border: '1px solid rgba(255, 255, 255, 0.05)'
                      }}
                    >
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <span style={{ fontWeight: 600, fontSize: '0.9rem' }}>{st.name}</span>
                        <span style={{
                          padding: '0.2rem 0.6rem',
                          borderRadius: '0.375rem',
                          fontSize: '0.75rem',
                          fontWeight: 700,
                          background: st.status === 'SUCCEEDED' ? 'rgba(16, 185, 129, 0.2)' : st.status === 'FAILED' ? 'rgba(239, 68, 68, 0.2)' : 'rgba(59, 130, 246, 0.2)',
                          color: st.status === 'SUCCEEDED' ? '#10b981' : st.status === 'FAILED' ? '#ef4444' : '#60a5fa'
                        }}>
                          {st.status}
                        </span>
                      </div>
                      <div style={{ fontSize: '0.75rem', color: '#94a3b8', marginTop: '0.3rem' }}>
                        Attempts: {st.attempts} {st.details && `• ${st.details}`} {st.error && `• Error: ${st.error}`}
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              {/* Payment State Machine */}
              <div style={{
                background: 'rgba(0, 0, 0, 0.3)',
                padding: '1.25rem',
                borderRadius: '0.75rem',
                border: '1px solid rgba(255, 255, 255, 0.05)'
              }}>
                <h3 style={{ fontSize: '1rem', fontWeight: 700, marginBottom: '1rem', color: '#e2e8f0' }}>Payment State Machine</h3>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', marginBottom: '1.25rem', flexWrap: 'wrap' }}>
                  {['CREATED', 'PENDING', 'AUTHORIZED', 'CAPTURED'].map((state, i) => {
                    const isActive = paymentState === state;
                    return (
                      <React.Fragment key={state}>
                        <span style={{
                          padding: '0.35rem 0.7rem',
                          borderRadius: '0.375rem',
                          fontSize: '0.8rem',
                          fontWeight: 700,
                          background: isActive ? '#3b82f6' : 'rgba(255, 255, 255, 0.05)',
                          color: isActive ? '#ffffff' : '#94a3b8',
                          boxShadow: isActive ? '0 0 10px rgba(59, 130, 246, 0.5)' : 'none'
                        }}>
                          {state}
                        </span>
                        {i < 3 && <span style={{ color: '#64748b' }}>➔</span>}
                      </React.Fragment>
                    );
                  })}
                </div>
                <div style={{ fontSize: '0.85rem', color: '#94a3b8' }}>
                  Payment ID: <code style={{ color: '#38bdf8' }}>{paymentId}</code>
                  <br />
                  Decider Service: <span style={{ color: '#a78bfa' }}>payment-core:4011</span>
                  <br />
                  Idempotency Key: <code style={{ color: '#e2e8f0', fontSize: '0.75rem' }}>idem_{paymentId}</code>
                </div>
              </div>
            </div>

            {/* Double-Entry Ledger Preview */}
            {ledgerEntries.length > 0 && (
              <div style={{
                background: 'rgba(0, 0, 0, 0.3)',
                padding: '1.25rem',
                borderRadius: '0.75rem',
                border: '1px solid rgba(255, 255, 255, 0.05)'
              }}>
                <h3 style={{ fontSize: '1rem', fontWeight: 700, marginBottom: '0.75rem', color: '#e2e8f0' }}>
                  Double-Entry Ledger Audit
                </h3>
                <div style={{ overflowX: 'auto' }}>
                  <table style={{ width: '100%', textAlign: 'left', borderCollapse: 'collapse', fontSize: '0.85rem' }}>
                    <thead>
                      <tr style={{ color: '#94a3b8', borderBottom: '1px solid rgba(255,255,255,0.1)' }}>
                        <th style={{ padding: '0.5rem' }}>Entry Type</th>
                        <th style={{ padding: '0.5rem' }}>Account</th>
                        <th style={{ padding: '0.5rem' }}>Amount (Minor Units)</th>
                        <th style={{ padding: '0.5rem' }}>Currency</th>
                        <th style={{ padding: '0.5rem' }}>Reason</th>
                      </tr>
                    </thead>
                    <tbody>
                      {ledgerEntries.map((l, i) => (
                        <tr key={i} style={{ borderBottom: '1px solid rgba(255,255,255,0.05)' }}>
                          <td style={{ padding: '0.5rem', fontWeight: 700, color: l.type === 'DEBIT' ? '#f87171' : '#34d399' }}>
                            {l.type}
                          </td>
                          <td style={{ padding: '0.5rem', color: '#e2e8f0' }}>{l.account}</td>
                          <td style={{ padding: '0.5rem', color: '#e2e8f0' }}>{l.amount.toLocaleString()} poisha</td>
                          <td style={{ padding: '0.5rem', color: '#94a3b8' }}>{l.currency}</td>
                          <td style={{ padding: '0.5rem', color: '#94a3b8' }}>{l.reason}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {/* Activity Stream */}
            <div style={{
              background: 'rgba(0, 0, 0, 0.5)',
              padding: '1.25rem',
              borderRadius: '0.75rem',
              border: '1px solid rgba(255, 255, 255, 0.05)',
              fontFamily: 'monospace',
              fontSize: '0.8rem',
              color: '#94a3b8',
              maxHeight: '220px',
              overflowY: 'auto'
            }}>
              <div style={{ fontWeight: 700, color: '#f8fafc', marginBottom: '0.5rem', fontFamily: 'sans-serif' }}>
                Live Mesh Activity Log
              </div>
              {eventLogs.map((ev, i) => (
                <div key={i} style={{ margin: '0.2rem 0' }}>{ev}</div>
              ))}
            </div>
          </div>
        )}
      </section>
    </main>
  );
}
