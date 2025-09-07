#!/bin/bash 
git config --global user.email "amirkhan.ibragimov@nu.edu.kz"
git config --global user.name "Amirkhan"
echo "Git global configuration has been set successfully!"
echo "User Name:  $(git config --global user.name)"
echo "User Email: $(git config --global user.email)"