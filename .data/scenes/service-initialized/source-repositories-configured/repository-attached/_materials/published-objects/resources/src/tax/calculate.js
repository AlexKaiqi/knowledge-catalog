export function calculateTax(net, rate) {
  return Math.round(net * rate * 100) / 100;
}
