# ~/.profile: executed by Bourne-compatible login shells.

echo "bbbbb"
source /opt/hygondriver/env.sh
if [ "$BASH" ]; then
  if [ -f ~/.bashrc ]; then
    . ~/.bashrc
  fi
fi

mesg n 2> /dev/null || true