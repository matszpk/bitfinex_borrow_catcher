## Bitfinex Borrow Catcher

The simple program that allow to borrow funds in a lesser interest rate than automatically
done by same the Bitfinex exchange. The program just check orderbook of
the funding periodically or in realtime and borrow funds before an automatic mechanism of
the Bitfinex exchange.

WARNING: THIS PROGRAM DOES NOT HAVE ANY WARRANTY AND YOU ARE USING
THIS SOFTWARE AT YOUR OWN RISK.
AUTHORS DOES NOT TAKE ANY RESPONSIBILITY FOR YOUR LOSES.

### Usage

The program read `bbc_config.json` file that contains configuration:

```
{
    "authFile":"exauth",
    "passwordFile":"password",
    "currency":"UST",
    "autoLoanFetchPeriod":"20m",
    "autoLoanFetchShift":"period",
    "autoLoanFetchEndShift":"period",
    "minRateDifference":0.2,
    "minOrderAmount":150,
    "minRateDiffInAskToForceBorrow": 0.1,
    "realtime": false
}
```

The following fields are:

* "authFile" - path to file where api key and secret key is stored in encrypted form.
* "passwordFile" - path to file with hashed password.
* "currency" - currency symbol in Bitfinex (UST - USDt, USD, BTC).
* "autoLoanFetchPeriod" - period between an automatic borrow mechanism -
  currently should be '20m'.
* "autoLoanFetchShift" - shift after automatic borrow mechanism -
  currently should be '11m' or later.
* "autoLoanFetchEndShift" - shift before automatic borrow mechanism -
  currently should be '10m' or earlier.
* "minRateDifference" - minimal rate difference between current borrow and
  required better interest rate. '0.2' -
  better an interest rate should be 20% less than current.
* "minOrderAmount" - minimal order amount in dollars - should be 150.
* "minRateDiffInAskToForceBorrow" - minimal rate difference that force borrow before
  deadline before an automatic mechanism.
* "realtime" - true if you want realtime orderbook checking - or false if your system
  have some problem with realtime checking - recommended is false.

After preparing configuration, user should generate password file by using command:

```
./bitfinex_borrow_catcher generate <password-file>
```

After run this command, program ask you about a password.

After the first run, program just ask you about an API key and a secret key which
will be encrypted to auth file. Next runs does not cause this question.

Program can works without to terminal access, because can ignore HUP signal. You can
safely run program in background and exit from remote shell.
Program prints to standard error messages about borrows, current borrow interest rate
and errors and you can redirect that standard error output to file. Typically
program can be run by command (with stderr redirection and on background running).

```
./bitfinex_borrow_catcher 2> bbc.log &
```
