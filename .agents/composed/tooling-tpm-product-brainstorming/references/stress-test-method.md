# The Stress-Test Methodology

Reference for [`stress-test.md`](stress-test.md). The five-step fan-in that breaks an assumption before the market does.

## Step 1: Isolate the Assumption

State it explicitly. Not "our market is large" but "the total addressable market for B2B spend management software in German SMEs is €2.3B."

The more specific the assumption, the more testable it is. Vague assumptions are unfalsifiable  -  and therefore useless.

**Common assumption types:**
- **Market size**  -  TAM, SAM, SOM; growth rate; customer segments
- **Customer behavior**  -  willingness to pay, churn, expansion, referrals
- **Revenue model**  -  conversion rates, deal size, sales cycle, CAC
- **Competitive position**  -  moat durability, competitor response speed, switching cost
- **Execution**  -  team velocity, hire timeline, product timeline, operational scaling
- **Macro**  -  regulatory environment, economic conditions, technology availability

## Step 2: Find the Counter-Evidence

For every assumption, actively search for evidence that it's wrong.

Ask:
- Who has tried this and failed?
- What data contradicts this assumption?
- What does the bear case look like?
- If a smart skeptic was looking at this, what would they point to?
- What's the base rate for assumptions like this?

**Sources of counter-evidence:**
- Comparable companies that failed in adjacent markets
- Customer churn data from similar businesses
- Historical accuracy of similar forecasts
- Industry reports with conflicting data
- What competitors who tried this found

The goal isn't to find a reason to stop  -  it's to surface what you don't know.

## Step 3: Model the Downside

Most plans model the base case and the upside. Stress testing means modeling the downside explicitly.

**For quantitative assumptions (revenue, growth, conversion):**

| Scenario | Assumption Value | Probability | Impact |
|----------|-----------------|-------------|--------|
| Base case | [Original value] | ? | |
| Bear case | -30% | ? | |
| Stress case | -50% | ? | |
| Catastrophic | -80% | ? | |

Key question at each level: **Does the business survive? Does the plan make sense?**

**For qualitative assumptions (moat, product-market fit, team capability):**

- What's the earliest signal this assumption is wrong?
- How long would it take you to notice?
- What happens between when it breaks and when you detect it?

## Step 4: Calculate Sensitivity

Some assumptions matter more than others. Sensitivity analysis answers: **if this one assumption changes, how much does the outcome change?**

Example:
- If CAC doubles, how does that change runway?
- If churn goes from 5% to 10%, how does that change NRR in 24 months?
- If the deal cycle is 6 months instead of 3, how does that affect Q3 revenue?

High sensitivity = the assumption is a key lever. Wrong = big problem.

## Step 5: Propose the Hedge

For every high-risk assumption, there should be a hedge:

- **Validation hedge**  -  test it before betting on it (pilot, customer conversation, small experiment)
- **Contingency hedge**  -  if it's wrong, what's plan B?
- **Early warning hedge**  -  what's the leading indicator that would tell you it's breaking before it's too late to act?
