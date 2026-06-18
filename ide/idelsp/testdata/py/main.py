# Unstable Build LLC ("COMPANY") CONFIDENTIAL
#
# Unpublished Copyright (c) 2017-2026 Unstable Build, All Rights Reserved.
#
# NOTICE: All information contained herein is, and remains the property of COMPANY.
# The intellectual and technical concepts contained herein are proprietary to
# COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
# and are protected by trade secret or copyright law. Dissemination of this information
# or reproduction of this material is strictly forbidden unless prior written permission
# is obtained from COMPANY. Access to the source code contained herein is hereby
# forbidden to anyone except current COMPANY employees, managers or contractors who
# have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
#
# The copyright notice above does not evidence any actual or intended publication or
# disclosure of this source code, which includes information that is confidential and/or
# proprietary, and is a trade secret, of COMPANY.

import os


class Greeter:
    """Greeter greets people."""

    def __init__(self, name: str) -> None:
        self.name = name

    def greet(self) -> str:
        """greet returns a greeting message."""
        return "Hello, " + self.name + "!"


def add(a: int, b: int) -> int:
    """add adds two integers."""
    return a+b


def main() -> None:
    g = Greeter("World")
    print(g.greet())
    result = add(1, 2)
    print(result)


if __name__ == "__main__":
    main()
